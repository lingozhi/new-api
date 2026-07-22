package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var preCommitCapacityTestChannelID atomic.Int64

type failOnceOnResponsesTerminalWriter struct {
	gin.ResponseWriter
	failed bool
}

func (w *failOnceOnResponsesTerminalWriter) Write(data []byte) (int, error) {
	if !w.failed && bytes.Contains(data, []byte("event: response.failed")) {
		w.failed = true
		return 0, errors.New("temporary terminal write failure")
	}
	return w.ResponseWriter.Write(data)
}

func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func upstreamRelayServiceErrorForTest() *types.NewAPIError {
	err := types.NewErrorWithStatusCode(
		errors.New("Upstream request failed, please try again, 请重试 (Relay Service)"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	err.UpstreamStatusCode = http.StatusBadRequest
	return err
}

func preCommitStreamCapacityErrorForTest(statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New("We're currently experiencing high demand, which may cause temporary errors."),
		types.ErrorCode("server_error"),
		statusCode,
		types.ErrOptionWithPreCommitStreamCapacity(),
	)
}

func TestIsRetryableChannelError(t *testing.T) {
	cases := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{
			name: "upstream 503 retryable",
			err:  types.NewErrorWithStatusCode(errors.New("no available accounts"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable),
			want: true,
		},
		{
			name: "upstream 502 retryable",
			err:  types.NewErrorWithStatusCode(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway),
			want: true,
		},
		{
			name: "capability 403 retryable",
			err:  types.NewErrorWithStatusCode(errors.New("Image generation is not enabled for this group"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
			want: true,
		},
		{
			name: "internal 500 retryable",
			err:  types.NewErrorWithStatusCode(errors.New("boom"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError),
			want: true,
		},
		{
			name: "client 400 not retryable",
			err:  types.NewErrorWithStatusCode(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry()),
			want: false,
		},
		{
			name: "success 200 not retryable",
			err:  types.NewErrorWithStatusCode(errors.New("ok"), types.ErrorCodeBadResponseStatusCode, http.StatusOK),
			want: false,
		},
		{
			name: "nil error not retryable",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestContext()
			if got := isRetryableChannelError(c, tc.err); got != tc.want {
				t.Fatalf("isRetryableChannelError(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestShouldRetryAllowsTransientAffinityFailure(t *testing.T) {
	c := newTestContext()
	c.Set("channel_affinity_skip_retry_on_failure", true)
	err := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)

	if !shouldRetry(c, err, 1) {
		t.Fatal("expected transient 5xx from a sticky channel to fall back")
	}
}

func TestShouldRetryUsesUpstream429ProvenanceAfterStatusMapping(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("Upstream rate limit exceeded, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	err.UpstreamStatusCode = http.StatusTooManyRequests

	c := newTestContext()
	assert.True(t, isRetryableChannelError(c, err))
	assert.True(t, shouldRetry(c, err, 1))
	assert.False(t, shouldRetry(c, err, 0), "upstream 429 must still respect the configured retry budget")

	pinned := newTestContext()
	pinned.Set("specific_channel_id", 17)
	assert.False(t, isRetryableChannelError(pinned, err))
	assert.False(t, shouldRetry(pinned, err, 1), "a pinned request cannot switch channels")

	affinity := newTestContext()
	affinity.Set("channel_affinity_skip_retry_on_failure", true)
	assert.True(t, isRetryableChannelError(affinity, err), "upstream provenance must override a mapped 400 affinity decision")
	assert.True(t, shouldRetry(affinity, err, 1))

	mapped503 := types.NewErrorWithStatusCode(
		errors.New("Upstream rate limit exceeded, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusServiceUnavailable,
	)
	mapped503.UpstreamStatusCode = http.StatusTooManyRequests
	assert.True(t, isRetryableChannelError(affinity, mapped503))
	assert.True(t, shouldRetry(affinity, mapped503, 1), "429 retry behavior must not depend on the client-facing mapping")
}

func TestShouldRetryTreatsRelayService400AsTransient(t *testing.T) {
	err := upstreamRelayServiceErrorForTest()

	c := newTestContext()
	assert.True(t, isRetryableChannelError(c, err))
	assert.True(t, shouldRetry(c, err, 1))
	assert.False(t, shouldRetry(c, err, 0), "the exact transient error must still respect the retry budget")

	affinity := newTestContext()
	affinity.Set("channel_affinity_skip_retry_on_failure", true)
	assert.True(t, shouldRetry(affinity, err, 1), "an upstream relay outage must escape a sticky channel")

	pinned := newTestContext()
	pinned.Set("specific_channel_id", 31)
	assert.False(t, shouldRetry(pinned, err, 1), "an explicitly pinned request cannot switch channels")

	localCopy := types.NewErrorWithStatusCode(
		errors.New("Upstream request failed, please try again, 请重试 (Relay Service)"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	assert.False(t, shouldRetry(newTestContext(), localCopy, 1), "matching text without upstream HTTP provenance must stay terminal")

	genericUpstream400 := types.NewErrorWithStatusCode(
		errors.New("invalid request parameter: model"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	genericUpstream400.UpstreamStatusCode = http.StatusBadRequest
	assert.False(t, shouldRetry(newTestContext(), genericUpstream400, 1), "ordinary upstream 400 responses must not be retried")
}

func TestMarkAffinityColdStartForTransientUpstreamRetry(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("Upstream rate limit exceeded, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	err.UpstreamStatusCode = http.StatusTooManyRequests

	affinity := newTestContext()
	service.MarkChannelAffinityUsed(affinity, "gpt pro", 17)
	assert.False(t, service.ShouldSkipRetryAfterChannelAffinityFailure(affinity), "the default soft affinity rule remains retryable")
	info := &relaycommon.RelayInfo{}
	markAffinityColdStartForRetry(affinity, info, err)
	assert.True(t, info.AffinityColdStart, "switching away from a warm affinity channel must exempt the cold fallback from latency penalties")
	assert.True(t, common.GetContextKeyBool(affinity, constant.ContextKeyAffinityColdStart))

	relayServiceInfo := &relaycommon.RelayInfo{}
	markAffinityColdStartForRetry(affinity, relayServiceInfo, upstreamRelayServiceErrorForTest())
	assert.True(t, relayServiceInfo.AffinityColdStart,
		"switching away from a failed relay-service affinity must exempt the cold fallback from latency penalties")

	localRateLimit := types.NewErrorWithStatusCode(
		errors.New("local rate limit"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	localInfo := &relaycommon.RelayInfo{}
	markAffinityColdStartForRetry(affinity, localInfo, localRateLimit)
	assert.False(t, localInfo.AffinityColdStart, "a gateway-local 429 does not move the request to another upstream channel")

	nonAffinityInfo := &relaycommon.RelayInfo{}
	markAffinityColdStartForRetry(newTestContext(), nonAffinityInfo, err)
	assert.False(t, nonAffinityInfo.AffinityColdStart)
}

func TestProcessChannelErrorCoolsRelayService400Briefly(t *testing.T) {
	model.ClearChannelCooldownsForTest()
	t.Cleanup(model.ClearChannelCooldownsForTest)

	const channelID = 9012
	processChannelError(
		newTestContext(),
		*types.NewChannelError(channelID, 1, "relay-service", false, "key", false),
		upstreamRelayServiceErrorForTest(),
	)

	reason, expires, cooling := model.GetChannelCooldown(channelID)
	require.True(t, cooling)
	assert.Contains(t, reason, "retryable_transient")
	remaining := time.Until(time.Unix(expires, 0))
	assert.Greater(t, remaining, 4*time.Minute)
	assert.Less(t, remaining, 6*time.Minute)
}

func TestProcessChannelErrorCoolsMappedUpstream429ForTwoHours(t *testing.T) {
	model.ClearChannelCooldownsForTest()
	t.Cleanup(model.ClearChannelCooldownsForTest)

	const channelID = 9005
	err := types.NewErrorWithStatusCode(
		errors.New("Upstream rate limit exceeded, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	err.UpstreamStatusCode = http.StatusTooManyRequests

	processChannelError(
		newTestContext(),
		*types.NewChannelError(channelID, 1, "rate-limited", false, "key", false),
		err,
	)

	reason, expires, cooling := model.GetChannelCooldown(channelID)
	require.True(t, cooling)
	assert.Contains(t, reason, "upstream_rate_limit")
	remaining := time.Until(time.Unix(expires, 0))
	assert.Greater(t, remaining, 119*time.Minute)
	assert.Less(t, remaining, 121*time.Minute)
}

func TestProcessChannelErrorDoesNotDuplicateMultiKeyStreamCapacityCooldown(t *testing.T) {
	model.ClearChannelCooldownsForTest()
	t.Cleanup(model.ClearChannelCooldownsForTest)

	channelID := int(9_013_000 + preCommitCapacityTestChannelID.Add(1))
	c := newTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         channelID,
			ChannelIsMultiKey: true,
		},
		OriginModelName: "gpt-5.6-sol",
		StreamStatus:    relaycommon.NewStreamStatus(),
	}
	failureErr := errors.New("upstream responses stream failed: We're currently experiencing high demand, which may cause temporary errors.")
	info.StreamStatus.RecordError(failureErr.Error())
	info.StreamStatus.SetEndReasonWithSource(relaycommon.StreamEndReasonUpstreamFailed, failureErr, "upstream_precommit")

	service.ObserveStreamChannelQualityForRequest(c, info)
	processChannelError(
		c,
		*types.NewChannelError(channelID, 1, "multi-key-capacity", true, "busy-key", false),
		preCommitStreamCapacityErrorForTest(http.StatusServiceUnavailable),
	)

	_, _, cooling := model.GetChannelCooldown(channelID)
	assert.False(t, cooling, "one busy key must not trigger the controller's generic whole-channel cooldown")
}

func TestShouldRetryStopsOnSemanticContextLimitError(t *testing.T) {
	c := newTestContext()
	err := types.NewErrorWithStatusCode(
		errors.New("Your input exceeds the context window of this model. Please adjust your input and try again."),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)

	if shouldRetry(c, err, 2) {
		t.Fatal("expected context-window errors to stop retrying even when upstream reports 502")
	}
	if isRetryableChannelError(c, err) {
		t.Fatal("expected context-window errors not to trigger transient channel cooldown")
	}
}

func TestProcessChannelErrorDoesNotCooldownSemanticContextLimitError(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("Your input exceeds the context window of this model. Please adjust your input and try again."),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)

	if shouldCooldownForUpstreamError(err) {
		t.Fatal("expected semantic context errors not to trigger upstream cooldown")
	}
}

func TestIsRetryableChannelErrorSkipsSpecificChannel(t *testing.T) {
	c := newTestContext()
	c.Set("specific_channel_id", 5)
	err := types.NewErrorWithStatusCode(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	if isRetryableChannelError(c, err) {
		t.Fatalf("expected pinned specific channel to skip retry classification")
	}
}

// TestShouldRetrySkipsClientCanceled guards the prod bug where one client abort
// burned through every channel: doRequest surfaces context.Canceled as a
// channel-class error, and types.IsChannelError returns true unconditionally
// (before the retry-count gate), so the loop retried on channel after channel —
// each failing in milliseconds and each getting cooled for 5 minutes.
func TestShouldRetrySkipsClientCanceled(t *testing.T) {
	c := newTestContext()

	canceled := types.NewErrorWithStatusCode(
		fmt.Errorf("do request failed: %w", context.Canceled),
		types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	if shouldRetry(c, canceled, 3) {
		t.Fatal("a client-canceled request must not be retried onto other channels")
	}
	if !isClientCanceledError(canceled) {
		t.Fatal("isClientCanceledError must recognize a wrapped context.Canceled")
	}

	// Our own timeout is a real channel signal and must still fail over.
	timeout := types.NewErrorWithStatusCode(
		fmt.Errorf("do request failed: %w", context.DeadlineExceeded),
		types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	if isClientCanceledError(timeout) {
		t.Fatal("context.DeadlineExceeded must not be treated as a client cancellation")
	}
	if !shouldRetry(c, timeout, 3) {
		t.Fatal("an upstream timeout must still retry onto another channel")
	}
}

func TestUpstreamCapacityFallbackRequiresUncommittedTransientCapacityError(t *testing.T) {
	newResponsesInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeResponses}
	}
	upstream429 := types.NewErrorWithStatusCode(
		errors.New("Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	upstream429.UpstreamStatusCode = http.StatusTooManyRequests

	assert.True(t, isUpstreamRateLimitError(upstream429))
	assert.True(t, isFastUpstreamCapacityError(upstream429))
	assert.True(t, shouldUseUpstreamCapacityFallback(newTestContext(), newResponsesInfo(), upstream429))

	mapped429 := types.NewErrorWithStatusCode(
		errors.New("Upstream rate limit exceeded, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusServiceUnavailable,
	)
	mapped429.UpstreamStatusCode = http.StatusTooManyRequests
	assert.True(t, isUpstreamRateLimitError(mapped429), "client-facing status mappings must not hide the upstream 429")
	assert.True(t, isFastUpstreamCapacityError(mapped429))
	assert.True(t, shouldUseUpstreamCapacityFallback(newTestContext(), newResponsesInfo(), mapped429))

	distributor503 := types.WithOpenAIError(
		types.OpenAIError{
			Message: "No available channel for model gpt-5.6-sol under group gpt plus (distributor)",
			Type:    "new_api_error",
			Code:    string(types.ErrorCodeModelNotFound),
		},
		http.StatusServiceUnavailable,
	)
	distributor503.UpstreamStatusCode = http.StatusServiceUnavailable
	assert.True(t, isFastUpstreamCapacityError(distributor503), "an upstream distributor with no account capacity should try another channel")
	assert.True(t, shouldUseUpstreamCapacityFallback(newTestContext(), newResponsesInfo(), distributor503))

	provider503 := types.WithOpenAIError(
		types.OpenAIError{
			Message: "The requested model does not exist",
			Type:    "invalid_request_error",
			Code:    string(types.ErrorCodeModelNotFound),
		},
		http.StatusServiceUnavailable,
	)
	provider503.UpstreamStatusCode = http.StatusServiceUnavailable
	assert.False(t, isFastUpstreamCapacityError(provider503), "provider model errors are not transient distributor capacity failures")

	distributor404 := types.WithOpenAIError(
		types.OpenAIError{
			Message: "No available channel for model gpt-5.6-sol under group gpt plus (distributor)",
			Type:    "new_api_error",
			Code:    string(types.ErrorCodeModelNotFound),
		},
		http.StatusNotFound,
	)
	distributor404.UpstreamStatusCode = http.StatusNotFound
	assert.False(t, isFastUpstreamCapacityError(distributor404), "a 404 model capability error must stay within configured retries")

	generic503 := types.NewErrorWithStatusCode(
		errors.New("service unavailable"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusServiceUnavailable,
	)
	generic503.UpstreamStatusCode = http.StatusServiceUnavailable
	assert.False(t, isFastUpstreamCapacityError(generic503), "generic 503s may be slow outages and must stay within configured retries")

	local429 := types.NewErrorWithStatusCode(
		errors.New("local rate limit"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	assert.False(t, isUpstreamRateLimitError(local429), "local 429s must stay outside channel retry policy")
	assert.False(t, isFastUpstreamCapacityError(local429))
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), newResponsesInfo(), local429))

	local503 := types.NewErrorWithStatusCode(
		errors.New("No available channel for model gpt-5.6-sol"),
		types.ErrorCodeModelNotFound,
		http.StatusServiceUnavailable,
	)
	assert.False(t, isFastUpstreamCapacityError(local503), "gateway-local capacity errors must not expand upstream attempts")

	quota429 := types.NewOpenAIError(
		errors.New("You exceeded your current quota"),
		types.ErrorCode("insufficient_quota"),
		http.StatusTooManyRequests,
	)
	quota429.UpstreamStatusCode = http.StatusTooManyRequests
	assert.True(t, isUpstreamRateLimitError(quota429), "every genuine upstream 429 must switch away from the affected channel")
	assert.False(t, isFastUpstreamCapacityError(quota429), "structural quota errors must stay inside the configured retry budget")
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), newResponsesInfo(), quota429))

	imageInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), imageInfo, upstream429), "side-effecting generation routes must not expand attempts")
	nonStreamingInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), nonStreamingInfo, upstream429), "non-streaming responses may legitimately wait longer than the fallback header deadline")
	compactInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponsesCompact}
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), compactInfo, upstream429), "response compaction must not inherit the streaming response-header deadline")

	pinned := newTestContext()
	pinned.Set("specific_channel_id", 17)
	assert.False(t, shouldUseUpstreamCapacityFallback(pinned, newResponsesInfo(), upstream429), "pinned channel requests must not switch channels")

	committed := newTestContext()
	committed.Writer.WriteHeaderNow()
	assert.True(t, relayResponseCommitted(committed, newResponsesInfo(), upstream429))
	assert.False(t, shouldUseUpstreamCapacityFallback(committed, newResponsesInfo(), upstream429))
}

func TestUpstreamCapacityFallbackStopsAfterAttemptStreamData(t *testing.T) {
	apiErr := types.NewErrorWithStatusCode(
		errors.New("rate limited"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	apiErr.UpstreamStatusCode = http.StatusTooManyRequests
	streamStatus := relaycommon.NewStreamStatus()
	streamStatus.RecordDataReceived()
	info := &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeResponses, StreamStatus: streamStatus}

	assert.True(t, relayResponseCommitted(newTestContext(), info, apiErr))
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), info, apiErr))
}

func TestUpstreamCapacityFallbackRetriesUncommittedStreamCapacityFailure(t *testing.T) {
	apiErr := preCommitStreamCapacityErrorForTest(http.StatusBadRequest)
	info := &relaycommon.RelayInfo{
		IsStream:     true,
		RelayMode:    relayconstant.RelayModeResponses,
		StreamStatus: relaycommon.NewStreamStatus(),
	}
	info.StreamStatus.RecordDataReceived()
	c := newTestContext()

	assert.True(t, isFastUpstreamCapacityError(apiErr))
	assert.False(t, relayResponseCommitted(c, info, apiErr), "upstream data is not a downstream commit when the marked handler withheld it")
	assert.True(t, shouldUseUpstreamCapacityFallback(c, info, apiErr))
	assert.True(t, scheduleUpstreamCapacityFallback(c, info, &service.RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		Retry:      common.GetPointer(0),
	}, apiErr, 500*time.Millisecond, 0, 0, time.Time{}, time.Now()), "an immediate explicit stream capacity error should get one bounded fallback even when RetryTimes is zero")
	assert.True(t, scheduleUpstreamCapacityFallback(c, info, &service.RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		Retry:      common.GetPointer(0),
	}, apiErr, 20*time.Second, 0, 0, time.Time{}, time.Now()), "an output-free delayed SSE capacity error gets one bounded fallback")
	assert.False(t, scheduleUpstreamCapacityFallback(c, info, &service.RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		Retry:      common.GetPointer(0),
	}, apiErr, 20*time.Second, 1, 0, time.Time{}, time.Now()), "a delayed SSE capacity fallback must remain single-attempt")

	affinity := newTestContext()
	affinity.Set("channel_affinity_skip_retry_on_failure", true)
	service.MarkChannelAffinityUsed(affinity, "gpt pro", 17)
	assert.True(t, shouldUseUpstreamCapacityFallback(affinity, info, apiErr), "an uncommitted capacity failure must escape a sticky channel")
	assert.True(t, shouldRetry(affinity, apiErr, 1), "status mapping must not strand a capacity failure on an affinity channel")
	affinityInfo := &relaycommon.RelayInfo{}
	markAffinityColdStartForRetry(affinity, affinityInfo, apiErr)
	assert.True(t, affinityInfo.AffinityColdStart)

	c.Writer.WriteHeaderNow()
	assert.False(t, relayResponseCommitted(c, info, apiErr), "keepalives do not make an explicitly output-free capacity failure unsafe to retry")

	unmarked := types.NewOpenAIError(
		errors.New("We're currently experiencing high demand, which may cause temporary errors."),
		types.ErrorCode("server_error"),
		http.StatusServiceUnavailable,
	)
	assert.True(t, relayResponseCommitted(newTestContext(), info, unmarked), "an unmarked upstream event must retain the normal commit boundary")
	assert.False(t, isFastUpstreamCapacityError(unmarked), "message text without internal provenance must not expand retries")
	assert.False(t, shouldUseUpstreamCapacityFallback(newTestContext(), info, unmarked))
}

func TestPrepareUncommittedRelayErrorResponseClearsSSEHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("X-Codex-Turn-State", "failed-channel-state")

	err := preCommitStreamCapacityErrorForTest(http.StatusServiceUnavailable)
	prepareUncommittedRelayErrorResponse(c, err)
	c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})

	response := recorder.Result()
	assert.Equal(t, "application/json; charset=utf-8", response.Header.Get("Content-Type"))
	assert.Empty(t, response.Header.Get("Transfer-Encoding"))
	assert.Empty(t, response.Header.Get("Connection"))
	assert.Empty(t, response.Header.Get("X-Accel-Buffering"))
	assert.Empty(t, response.Header.Get("X-Codex-Turn-State"))
}

func TestWriteCommittedResponsesCapacityTerminalKeepsSSEFraming(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	_, err := c.Writer.Write([]byte(": PING\n\n"))
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		IsStream:     true,
		RelayMode:    relayconstant.RelayModeResponses,
		ChannelMeta:  &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-sol"},
		StreamStatus: relaycommon.NewStreamStatus(),
	}
	apiErr := preCommitStreamCapacityErrorForTest(http.StatusServiceUnavailable)

	assert.True(t, writeCommittedResponsesCapacityTerminal(c, info, apiErr))
	body := recorder.Body.String()
	assert.True(t, strings.HasPrefix(body, ": PING\n\n"))
	assert.Contains(t, body, "event: response.failed")
	assert.Contains(t, body, "We're currently experiencing high demand")
	assert.NotContains(t, body, "\n{\"error\":", "a committed SSE stream must never receive a naked JSON error body")
	assert.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestWriteCommittedResponsesCapacityTerminalRetriesPendingTerminalOnce(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	_, err := c.Writer.Write([]byte(": PING\n\n"))
	require.NoError(t, err)
	c.Writer = &failOnceOnResponsesTerminalWriter{ResponseWriter: c.Writer}

	info := &relaycommon.RelayInfo{
		IsStream:     true,
		RelayMode:    relayconstant.RelayModeResponses,
		ChannelMeta:  &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-sol"},
		StreamStatus: relaycommon.NewStreamStatus(),
	}

	assert.True(t, writeCommittedResponsesCapacityTerminal(c, info, preCommitStreamCapacityErrorForTest(http.StatusServiceUnavailable)))
	body := recorder.Body.String()
	assert.True(t, strings.HasPrefix(body, ": PING\n\n"))
	assert.Equal(t, 1, strings.Count(body, "event: response.failed"))
	assert.Contains(t, body, "We're currently experiencing high demand")
	assert.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestScheduleUpstreamCapacityFallbackIsBoundedAndRestartsAutoSelection(t *testing.T) {
	c := newTestContext()
	apiErr := types.NewErrorWithStatusCode(
		errors.New("Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	apiErr.UpstreamStatusCode = http.StatusTooManyRequests
	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		Retry:      common.GetPointer(0),
	}
	startedAt := time.Unix(100, 0)

	info := &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeResponses}
	delayedCapacity := preCommitStreamCapacityErrorForTest(http.StatusServiceUnavailable)
	assert.True(t, shouldEvaluateUpstreamCapacityFallback(false, 0, 3, delayedCapacity, 20*time.Second),
		"a delayed output-free capacity error must bypass a larger normal retry budget and stay single-attempt")
	assert.False(t, shouldEvaluateUpstreamCapacityFallback(false, 0, 3, delayedCapacity, 500*time.Millisecond))
	assert.True(t, scheduleUpstreamCapacityFallback(c, info, retryParam, apiErr, 500*time.Millisecond, 0, 2, time.Time{}, startedAt))
	retryParam.IncreaseRetry()
	assert.Equal(t, 2, retryParam.GetRetry())

	assert.True(t, scheduleUpstreamCapacityFallback(c, info, retryParam, apiErr, 400*time.Millisecond, 1, 2, startedAt, startedAt.Add(2*time.Second)),
		"a second fast capacity failure may try one more untried channel")
	assert.False(t, scheduleUpstreamCapacityFallback(c, info, retryParam, apiErr, 300*time.Millisecond, 2, 2, startedAt, startedAt.Add(3*time.Second)),
		"capacity fallback attempts must have a hard count limit")
	assert.False(t, scheduleUpstreamCapacityFallback(c, info, retryParam, apiErr, 300*time.Millisecond, 1, 2, startedAt, startedAt.Add(6*time.Second)),
		"a slow fallback attempt must not expand first-token latency with another channel")
	assert.True(t, scheduleUpstreamCapacityFallback(c, info, retryParam, apiErr, 3*time.Second, 0, 2, time.Time{}, startedAt),
		"a delayed genuine upstream 429 must still be shielded from Codex")
	assert.True(t, scheduleUpstreamCapacityFallback(c, info, retryParam, delayedCapacity, 20*time.Second, 0, 2, time.Time{}, startedAt),
		"a delayed output-free SSE capacity terminal gets one bounded fallback")
	assert.False(t, scheduleUpstreamCapacityFallback(c, info, retryParam, delayedCapacity, 20*time.Second, 1, 2, time.Time{}, startedAt),
		"a delayed output-free SSE fallback cannot expand into a second attempt")

	distributor503 := types.WithOpenAIError(
		types.OpenAIError{
			Message: "No available channel for model gpt-5.6-sol under group gpt plus (distributor)",
			Type:    "new_api_error",
			Code:    string(types.ErrorCodeModelNotFound),
		},
		http.StatusServiceUnavailable,
	)
	distributor503.UpstreamStatusCode = http.StatusServiceUnavailable
	assert.False(t, scheduleUpstreamCapacityFallback(c, info, retryParam, distributor503, 3*time.Second, 0, 2, time.Time{}, startedAt),
		"a slow distributor 503 must not expand first-token latency")
}

func TestCapacityFallbackAffinityResumeRequiresSuccessfulTerminal(t *testing.T) {
	for _, reason := range []relaycommon.StreamEndReason{
		relaycommon.StreamEndReasonUpstreamFailed,
		relaycommon.StreamEndReasonTimeout,
		relaycommon.StreamEndReasonScannerErr,
		relaycommon.StreamEndReasonPingFail,
		relaycommon.StreamEndReasonInternalError,
		relaycommon.StreamEndReasonTerminalClientError,
	} {
		info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
		info.StreamStatus.SetEndReason(reason, errors.New("fallback terminal"))
		assert.False(t, capacityFallbackSucceeded(info), "terminal %s must remain suppressed", reason)
	}

	for _, reason := range []relaycommon.StreamEndReason{
		relaycommon.StreamEndReasonDone,
		relaycommon.StreamEndReasonEOF,
	} {
		info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
		info.StreamStatus.SetEndReason(reason, nil)
		assert.True(t, capacityFallbackSucceeded(info), "terminal %s is a successful fallback", reason)
	}
	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, nil)
	assert.False(t, capacityFallbackSucceeded(info), "handler_stop is ambiguous and must not resume affinity")
	info = &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, errors.New("scanner eof after failure"))
	assert.False(t, capacityFallbackSucceeded(info), "EOF with an error must not resume affinity")
}

func TestChannelSelectionExhaustionIsDistinctFromSelectorFailure(t *testing.T) {
	exhausted := types.NewError(&channelSelectionExhaustedError{message: "no untried channel"}, types.ErrorCodeGetChannelFailed)
	selectorFailure := types.NewError(errors.New("database unavailable"), types.ErrorCodeGetChannelFailed)

	assert.True(t, isChannelSelectionExhausted(exhausted))
	assert.Equal(t, "no untried channel", exhausted.Error())
	assert.False(t, isChannelSelectionExhausted(selectorFailure))
}

func TestCapacityFallbackHeaderDeadlinePreservesSecondAttempt(t *testing.T) {
	startedAt := time.Unix(100, 0)

	firstDeadline := capacityFallbackHeaderDeadline(startedAt, startedAt.Add(500*time.Millisecond), 1)
	assert.Equal(t, startedAt.Add(2500*time.Millisecond), firstDeadline)
	assert.True(t, canContinueCapacityFallbackAfterHeaderDeadline(1, startedAt, firstDeadline))

	secondDeadline := capacityFallbackHeaderDeadline(startedAt, firstDeadline, 2)
	assert.Equal(t, startedAt.Add(upstreamCapacityFallbackWindow), secondDeadline)
	assert.False(t, canContinueCapacityFallbackAfterHeaderDeadline(2, startedAt, firstDeadline))

	nearOverallDeadline := capacityFallbackHeaderDeadline(startedAt, startedAt.Add(4*time.Second), 1)
	assert.Equal(t, startedAt.Add(upstreamCapacityFallbackWindow), nearOverallDeadline)
	assert.False(t, canContinueCapacityFallbackAfterHeaderDeadline(1, startedAt, startedAt.Add(upstreamCapacityFallbackWindow)))
}
