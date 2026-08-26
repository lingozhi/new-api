package service

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type webhookRoundTripFunc func(*http.Request) (*http.Response, error)

func (f webhookRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestWebhookChallengeMemoryRateLimiterCountsAttemptsByUserAndToken(t *testing.T) {
	const (
		now           = int64(1_000)
		windowSeconds = int64(60)
	)

	t.Run("a failed network attempt still consumes the token allowance", func(t *testing.T) {
		limiter := webhookChallengeMemoryRateLimiter{}
		subjects := []webhookChallengeRateLimitSubject{
			{key: "user:1", limit: 10},
			{key: "token:1", limit: 1},
		}
		retryAfter, err := limiter.allow(now, windowSeconds, subjects...)
		require.NoError(t, err)
		assert.Zero(t, retryAfter)

		// Admission happens before URL validation and the outbound POST. Treat the
		// first allowed call as failed; the next call must still be blocked.
		retryAfter, err = limiter.allow(now, windowSeconds, subjects...)
		require.NoError(t, err)
		assert.Equal(t, int(windowSeconds), retryAfter)
	})

	t.Run("rotating tokens cannot bypass the aggregate user limit", func(t *testing.T) {
		limiter := webhookChallengeMemoryRateLimiter{}
		first := []webhookChallengeRateLimitSubject{
			{key: "user:2", limit: 1},
			{key: "token:2", limit: 10},
		}
		retryAfter, err := limiter.allow(now, windowSeconds, first...)
		require.NoError(t, err)
		assert.Zero(t, retryAfter)

		second := []webhookChallengeRateLimitSubject{
			{key: "user:2", limit: 1},
			{key: "token:3", limit: 10},
		}
		retryAfter, err = limiter.allow(now, windowSeconds, second...)
		require.NoError(t, err)
		assert.Equal(t, int(windowSeconds), retryAfter)
	})
}

func TestWebhookChallengeRedisRateLimiterIsSharedAndCountsFailures(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	previousRDB := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousRDB })
	subjects := []webhookChallengeRateLimitSubject{
		{key: "rateLimit:webhookChallenge:user:redis-test", limit: 10},
		{key: "rateLimit:webhookChallenge:token:redis-test", limit: 1},
	}

	retryAfter, err := allowWebhookChallengeRedis(context.Background(), subjects, 60)
	require.NoError(t, err)
	assert.Zero(t, retryAfter)

	// A second process would execute the same script against these keys. The
	// first attempt's eventual challenge outcome does not remove its count.
	retryAfter, err = allowWebhookChallengeRedis(context.Background(), subjects, 60)
	require.NoError(t, err)
	assert.Equal(t, 60, retryAfter)

	server.FastForward(60 * time.Second)
	retryAfter, err = allowWebhookChallengeRedis(context.Background(), subjects, 60)
	require.NoError(t, err)
	assert.Zero(t, retryAfter)
}

func TestWebhookChallengeAdmissionFallsBackToMemoryAndRejectsAtCapacity(t *testing.T) {
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	webhookChallengeMemoryLimiter = webhookChallengeMemoryRateLimiter{}
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		webhookChallengeMemoryLimiter = webhookChallengeMemoryRateLimiter{}
	})

	for attempt := 0; attempt < webhookChallengeTokenRateLimit; attempt++ {
		retryAfter, err := allowWebhookChallengeAttempt(context.Background(), 8_001, 9_001)
		require.NoError(t, err)
		assert.Zero(t, retryAfter)
	}
	retryAfter, err := allowWebhookChallengeAttempt(context.Background(), 8_001, 9_001)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, retryAfter, 1)
	assert.LessOrEqual(t, retryAfter, int(webhookChallengeRateLimitWindow/time.Second))

	previousSlots := webhookChallengeSlots
	webhookChallengeSlots = make(chan struct{}, 1)
	webhookChallengeSlots <- struct{}{}
	t.Cleanup(func() { webhookChallengeSlots = previousSlots })

	release, err := acquireWebhookChallengeAdmission(context.Background(), 8_002, 9_002)
	require.Error(t, err)
	assert.Nil(t, release)
	retryAfter, limited := WebhookChallengeRetryAfter(err)
	assert.True(t, limited)
	assert.Equal(t, 1, retryAfter)

	<-webhookChallengeSlots
	release, err = acquireWebhookChallengeAdmission(context.Background(), 8_003, 9_003)
	require.NoError(t, err)
	require.NotNil(t, release)
	assert.Len(t, webhookChallengeSlots, 1)
	release()
	assert.Empty(t, webhookChallengeSlots)
}

func TestVerifyJSONWebhookChallengeWithClientEchoesChallenge(t *testing.T) {
	const challenge = "challenge_123"
	var received webhookChallenge
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, common.DecodeJson(r.Body, &received))

		response, err := common.Marshal(webhookChallenge{Challenge: received.Challenge})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(response)
		require.NoError(t, err)
	}))
	defer server.Close()

	require.NoError(t, verifyJSONWebhookChallengeWithClient(context.Background(), server.Client(), server.URL, challenge))
	assert.Equal(t, challenge, received.Challenge)
}

func TestVerifyJSONWebhookChallengeWithClientRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		response    []byte
		errContains string
	}{
		{
			name:        "non-success status",
			status:      http.StatusBadGateway,
			response:    []byte(`{"challenge":"challenge_123"}`),
			errContains: "502",
		},
		{
			name:        "malformed JSON",
			status:      http.StatusOK,
			response:    []byte(`{"challenge":`),
			errContains: "invalid webhook challenge response",
		},
		{
			name:        "mismatched challenge",
			status:      http.StatusOK,
			response:    []byte(`{"challenge":"different"}`),
			errContains: "did not echo",
		},
		{
			name:        "oversized response",
			status:      http.StatusOK,
			response:    make([]byte, webhookChallengeMaxResponseBytes+1),
			errContains: "exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, err := w.Write(test.response)
				require.NoError(t, err)
			}))
			defer server.Close()

			err := verifyJSONWebhookChallengeWithClient(context.Background(), server.Client(), server.URL, "challenge_123")
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.errContains)
		})
	}
}

func TestVerifyJSONWebhookChallengeWithClientRejectsRedirect(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	err := verifyJSONWebhookChallengeWithClient(context.Background(), redirect.Client(), redirect.URL, "challenge_123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "307")
	assert.False(t, redirected)
}

func TestVerifyJSONWebhookChallengeWithClientHonorsContextDeadline(t *testing.T) {
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := verifyJSONWebhookChallengeWithClient(ctx, server.Client(), server.URL, "challenge_123")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestJSONWebhookRequestsKeepTLSVerificationEnabled(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/challenge" {
			var request webhookChallenge
			require.NoError(t, common.DecodeJson(r.Body, &request))
			response, err := common.Marshal(request)
			require.NoError(t, err)
			_, err = w.Write(response)
			require.NoError(t, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()

	originalTLSInsecureSkipVerify := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	testClient := newStrictDirectProtectedFetchHTTPClient()
	protectedTransport := testClient.Transport.(*ssrfProtectedRoundTripper)
	localAddress := server.Listener.Addr().String()
	protectedTransport.dialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, localAddress)
	}

	proxyClientLock.Lock()
	originalClient := strictDirectMediaHTTPClient
	strictDirectMediaHTTPClient = testClient
	proxyClientLock.Unlock()
	t.Cleanup(func() {
		proxyClientLock.Lock()
		strictDirectMediaHTTPClient = originalClient
		proxyClientLock.Unlock()
		common.TLSInsecureSkipVerify = originalTLSInsecureSkipVerify
	})

	err := VerifyJSONWebhookChallenge(context.Background(), "https://8.8.8.8/challenge", 8_101, 9_101)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate")

	err = SendJSONWebhookWithDeliveryID(context.Background(), "https://8.8.8.8/delivery", "", "task_123", map[string]string{"status": "completed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate")
}

func TestSendJSONWebhookWithClientSignsExactPayload(t *testing.T) {
	secret := "webhook-secret"
	payload := map[string]any{
		"task_id": "task_123",
		"status":  "completed",
	}

	var receivedBody []byte
	var receivedSignature string
	var receivedDeliveryID string
	var receivedTimestamp string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedSignature = r.Header.Get("X-Webhook-Signature")
		receivedDeliveryID = r.Header.Get(WebhookDeliveryIDHeader)
		receivedTimestamp = r.Header.Get(WebhookTimestampHeader)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	require.NoError(t, sendJSONWebhookWithClient(context.Background(), server.Client(), server.URL, secret, "task_123", payload))

	var decoded map[string]any
	require.NoError(t, common.Unmarshal(receivedBody, &decoded))
	assert.Equal(t, "task_123", decoded["task_id"])

	assert.NotEmpty(t, receivedTimestamp)
	assert.Equal(t, generateVersionedSignature(secret, receivedTimestamp, "task_123", receivedBody), receivedSignature)
	assert.NotEqual(t, generateVersionedSignature(secret, receivedTimestamp, "task_changed", receivedBody), receivedSignature)
	assert.Equal(t, "task_123", receivedDeliveryID)
}

func TestSendJSONWebhookWithClientPreservesLegacyBodySignatureWithoutDeliveryID(t *testing.T) {
	secret := "legacy-webhook-secret"
	payload := map[string]string{"status": "ok"}
	var receivedBody []byte
	var receivedSignature string
	var receivedTimestamp string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedSignature = r.Header.Get("X-Webhook-Signature")
		receivedTimestamp = r.Header.Get(WebhookTimestampHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	require.NoError(t, sendJSONWebhookWithClient(context.Background(), server.Client(), server.URL, secret, "", payload))
	assert.Equal(t, generateSignature(secret, receivedBody), receivedSignature)
	assert.Empty(t, receivedTimestamp)
}

func TestSendJSONWebhookWithClientRejectsNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	err := sendJSONWebhookWithClient(context.Background(), server.Client(), server.URL, "", "task_failed", map[string]string{"status": "failed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestJSONWebhookTransportErrorsDoNotExposeCallbackCredentials(t *testing.T) {
	cause := errors.New("connection refused")
	client := &http.Client{Transport: webhookRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	})}
	callbackURL := "https://callbacks.example.com/minimax?access_token=top-secret"

	err := sendJSONWebhookWithClient(context.Background(), client, callbackURL, "", "task_secret", map[string]string{"status": "failed"})
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), "top-secret")
	assert.Contains(t, err.Error(), "access_token=***")

	err = verifyJSONWebhookChallengeWithClient(context.Background(), client, callbackURL, "challenge_123")
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), "top-secret")
	assert.Contains(t, err.Error(), "access_token=***")
}

func TestSendJSONWebhookWithClientRejectsRedirectWithoutForwardingSignature(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		assert.Empty(t, r.Header.Get("X-Webhook-Signature"))
		assert.Empty(t, r.Header.Get(WebhookDeliveryIDHeader))
		assert.Empty(t, r.Header.Get(WebhookTimestampHeader))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	err := sendJSONWebhookWithClient(context.Background(), redirect.Client(), redirect.URL, "secret", "task_redirect", map[string]string{"status": "completed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "302")
	assert.False(t, redirected)
}

func TestSendJSONWebhookWithClientHonorsContextDeadline(t *testing.T) {
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := sendJSONWebhookWithClient(ctx, server.Client(), server.URL, "", "task_timeout", map[string]string{"status": "completed"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestValidateJSONWebhookURLAlwaysRequiresHTTPS(t *testing.T) {
	oldWorkerURL := system_setting.WorkerUrl
	oldAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	system_setting.WorkerUrl = "https://worker.example.com"
	system_setting.WorkerAllowHttpImageRequestEnabled = false
	t.Cleanup(func() {
		system_setting.WorkerUrl = oldWorkerURL
		system_setting.WorkerAllowHttpImageRequestEnabled = oldAllowHTTP
	})

	err := ValidateJSONWebhookURL("http://8.8.8.8/webhook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
	require.NoError(t, ValidateJSONWebhookURL("https://8.8.8.8/webhook"))

	system_setting.WorkerAllowHttpImageRequestEnabled = true
	err = ValidateJSONWebhookURL("http://8.8.8.8/webhook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

func TestValidateJSONWebhookURLRejectsPrivateTargetsWhenGeneralProtectionDisabled(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.AllowedPorts = []string{"80", "443"}
	fetchSetting.ApplyIPFilterForDomain = false
	t.Cleanup(func() { *fetchSetting = original })

	err := ValidateJSONWebhookURL("https://127.0.0.1/webhook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private IP address not allowed")
	require.NoError(t, ValidateJSONWebhookURL("https://8.8.8.8/webhook"))
}
