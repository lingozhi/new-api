package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// WebhookPayload webhook 通知的负载数据
type WebhookPayload struct {
	Type      string        `json:"type"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Values    []interface{} `json:"values,omitempty"`
	Timestamp int64         `json:"timestamp"`
}

const (
	webhookChallengeTimeout          = 3 * time.Second
	webhookChallengeMaxResponseBytes = 64 * 1024
	webhookChallengeRateLimitWindow  = time.Minute
	webhookChallengeUserRateLimit    = 60
	webhookChallengeTokenRateLimit   = 30
	webhookChallengeMaxConcurrent    = 16
	webhookDeliveryTimeout           = 15 * time.Second
	WebhookDeliveryIDHeader          = "X-Webhook-Delivery-Id"
	WebhookTimestampHeader           = "X-Webhook-Timestamp"
	webhookSignatureVersion          = "v1"
)

const webhookChallengeRateLimitScript = `
local function increment_and_retry(key, limit, duration)
  local current = redis.call('INCR', key)
  local ttl = redis.call('TTL', key)
  if current == 1 or ttl < 0 then
    redis.call('EXPIRE', key, duration)
    ttl = duration
  end
  if current > limit then
    if ttl < 1 then
      ttl = 1
    end
    return ttl
  end
  return 0
end

local user_retry = increment_and_retry(KEYS[1], tonumber(ARGV[1]), tonumber(ARGV[3]))
local token_retry = increment_and_retry(KEYS[2], tonumber(ARGV[2]), tonumber(ARGV[3]))
if token_retry > user_retry then
  return token_retry
end
return user_retry
`

var ErrWebhookChallengePrincipalUnavailable = errors.New("authenticated webhook challenge principal is unavailable")

type WebhookChallengeAdmissionError struct {
	RetryAfterSeconds int
	reason            string
}

func (e *WebhookChallengeAdmissionError) Error() string {
	if e == nil || e.reason == "" {
		return "webhook challenge admission limit exceeded"
	}
	return e.reason
}

func WebhookChallengeRetryAfter(err error) (int, bool) {
	var admissionErr *WebhookChallengeAdmissionError
	if !errors.As(err, &admissionErr) {
		return 0, false
	}
	retryAfter := admissionErr.RetryAfterSeconds
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter, true
}

type webhookChallengeRateLimitEntry struct {
	attempts int
	resetAt  int64
}

type webhookChallengeRateLimitSubject struct {
	key   string
	limit int
}

type webhookChallengeMemoryRateLimiter struct {
	mutex       sync.Mutex
	entries     map[string]webhookChallengeRateLimitEntry
	lastCleanup int64
}

func (l *webhookChallengeMemoryRateLimiter) allow(now int64, windowSeconds int64, subjects ...webhookChallengeRateLimitSubject) (int, error) {
	if windowSeconds < 1 || len(subjects) == 0 {
		return 0, errors.New("webhook challenge rate-limit configuration is invalid")
	}
	for _, subject := range subjects {
		if subject.key == "" || subject.limit < 1 {
			return 0, errors.New("webhook challenge rate-limit subject is invalid")
		}
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.entries == nil {
		l.entries = make(map[string]webhookChallengeRateLimitEntry)
	}
	if l.lastCleanup == 0 || now-l.lastCleanup >= windowSeconds {
		for key, entry := range l.entries {
			if entry.resetAt <= now {
				delete(l.entries, key)
			}
		}
		l.lastCleanup = now
	}

	retryAfter := int64(0)
	for _, subject := range subjects {
		entry := l.entries[subject.key]
		if entry.resetAt <= now {
			entry = webhookChallengeRateLimitEntry{resetAt: now + windowSeconds}
		}
		// Once blocked, retaining limit+1 is enough to count later failures while
		// avoiding integer growth under a sustained abusive request stream.
		if entry.attempts <= subject.limit {
			entry.attempts++
		}
		l.entries[subject.key] = entry
		if entry.attempts > subject.limit {
			remaining := entry.resetAt - now
			if remaining < 1 {
				remaining = 1
			}
			if remaining > retryAfter {
				retryAfter = remaining
			}
		}
	}
	return int(retryAfter), nil
}

var (
	webhookChallengeMemoryLimiter webhookChallengeMemoryRateLimiter
	webhookChallengeSlots         = make(chan struct{}, webhookChallengeMaxConcurrent)
)

type webhookChallenge struct {
	Challenge string `json:"challenge"`
}

type sanitizedWebhookTransportError struct {
	cause error
}

func (e sanitizedWebhookTransportError) Error() string {
	return common.MaskSensitiveInfo(e.cause.Error())
}

func (e sanitizedWebhookTransportError) Unwrap() error {
	return e.cause
}

// generateSignature preserves the existing notification-webhook contract.
func generateSignature(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func generateVersionedSignature(secret string, timestamp string, deliveryID string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(webhookSignatureVersion))
	h.Write([]byte("."))
	h.Write([]byte(timestamp))
	h.Write([]byte("."))
	h.Write([]byte(deliveryID))
	h.Write([]byte("."))
	h.Write(payload)
	return webhookSignatureVersion + "=" + hex.EncodeToString(h.Sum(nil))
}

// ValidateJSONWebhookURL applies the SSRF policy and requires encrypted
// transport for async callback payloads and signatures.
func ValidateJSONWebhookURL(webhookURL string) error {
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("async task webhook transport only supports https URLs")
	}
	return validateWebhookFetchURL(webhookURL)
}

func webhookChallengeRateLimitSubjects(userID int, tokenID int) ([]webhookChallengeRateLimitSubject, error) {
	if userID <= 0 || tokenID <= 0 {
		return nil, ErrWebhookChallengePrincipalUnavailable
	}
	return []webhookChallengeRateLimitSubject{
		{key: "rateLimit:webhookChallenge:user:" + strconv.Itoa(userID), limit: webhookChallengeUserRateLimit},
		{key: "rateLimit:webhookChallenge:token:" + strconv.Itoa(tokenID), limit: webhookChallengeTokenRateLimit},
	}, nil
}

func allowWebhookChallengeRedis(ctx context.Context, subjects []webhookChallengeRateLimitSubject, windowSeconds int64) (int, error) {
	if common.RDB == nil || len(subjects) != 2 || windowSeconds < 1 {
		return 0, errors.New("webhook challenge Redis rate limiter is unavailable")
	}
	retryAfter, err := common.RDB.Eval(
		ctx,
		webhookChallengeRateLimitScript,
		[]string{subjects[0].key, subjects[1].key},
		subjects[0].limit,
		subjects[1].limit,
		windowSeconds,
	).Int64()
	if err != nil {
		return 0, err
	}
	if retryAfter < 0 {
		return 0, errors.New("webhook challenge Redis rate limiter returned an invalid retry delay")
	}
	return int(retryAfter), nil
}

func allowWebhookChallengeAttempt(ctx context.Context, userID int, tokenID int) (int, error) {
	subjects, err := webhookChallengeRateLimitSubjects(userID, tokenID)
	if err != nil {
		return 0, err
	}
	windowSeconds := int64(webhookChallengeRateLimitWindow / time.Second)
	if common.RedisEnabled && common.RDB != nil {
		if retryAfter, redisErr := allowWebhookChallengeRedis(ctx, subjects, windowSeconds); redisErr == nil {
			return retryAfter, nil
		}
	}
	return webhookChallengeMemoryLimiter.allow(time.Now().Unix(), windowSeconds, subjects...)
}

func acquireWebhookChallengeAdmission(ctx context.Context, userID int, tokenID int) (func(), error) {
	retryAfter, err := allowWebhookChallengeAttempt(ctx, userID, tokenID)
	if err != nil {
		return nil, err
	}
	if retryAfter > 0 {
		return nil, &WebhookChallengeAdmissionError{
			RetryAfterSeconds: retryAfter,
			reason:            "webhook challenge rate limit exceeded",
		}
	}

	select {
	case webhookChallengeSlots <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-webhookChallengeSlots })
		}, nil
	default:
		return nil, &WebhookChallengeAdmissionError{
			RetryAfterSeconds: 1,
			reason:            "webhook challenge capacity is temporarily unavailable",
		}
	}
}

// VerifyJSONWebhookChallenge validates ownership of a callback endpoint before
// a task is submitted. The endpoint must echo the random challenge in a JSON
// response within the provider-compatible three-second window. Admission is
// always counted before validation or network I/O, so failed probes cannot be
// used as a free outbound-request primitive.
func VerifyJSONWebhookChallenge(ctx context.Context, webhookURL string, userID int, tokenID int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := acquireWebhookChallengeAdmission(ctx, userID, tokenID)
	if err != nil {
		return err
	}
	defer release()

	requestCtx, cancel := context.WithTimeout(ctx, webhookChallengeTimeout)
	defer cancel()

	if err := ValidateJSONWebhookURL(webhookURL); err != nil {
		return fmt.Errorf("request reject: %w", err)
	}

	return verifyJSONWebhookChallengeWithClient(
		requestCtx,
		GetStrictHTTPSDirectSSRFProtectedHTTPClient(),
		webhookURL,
		common.GetUUID(),
	)
}

func verifyJSONWebhookChallengeWithClient(ctx context.Context, client *http.Client, webhookURL string, challenge string) error {
	if client == nil {
		return fmt.Errorf("webhook HTTP client is required")
	}
	if challenge == "" {
		return fmt.Errorf("webhook challenge is required")
	}

	payload, err := common.Marshal(webhookChallenge{Challenge: challenge})
	if err != nil {
		return fmt.Errorf("failed to marshal webhook challenge: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook challenge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	challengeClient := *client
	challengeClient.Timeout = 0
	challengeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := challengeClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook challenge: %w", sanitizedWebhookTransportError{cause: err})
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook challenge failed with status code: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, webhookChallengeMaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read webhook challenge response: %w", err)
	}
	if len(responseBody) > webhookChallengeMaxResponseBytes {
		return fmt.Errorf("webhook challenge response exceeds %d bytes", webhookChallengeMaxResponseBytes)
	}

	var response webhookChallenge
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("invalid webhook challenge response: %w", err)
	}
	if response.Challenge != challenge {
		return fmt.Errorf("webhook challenge response did not echo the challenge")
	}
	return nil
}

// SendJSONWebhook preserves the original body-only signature contract.
func SendJSONWebhook(ctx context.Context, webhookURL string, secret string, payload any) error {
	return SendJSONWebhookWithDeliveryID(ctx, webhookURL, secret, "", payload)
}

// SendJSONWebhookWithDeliveryID sends an at-least-once webhook delivery. The
// delivery ID must remain stable across retries so receivers can deduplicate a
// request when an acknowledgement is lost after they process it.
func SendJSONWebhookWithDeliveryID(ctx context.Context, webhookURL string, secret string, deliveryID string, payload any) error {
	requestCtx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout)
	defer cancel()

	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}
	if err := ValidateJSONWebhookURL(webhookURL); err != nil {
		return fmt.Errorf("request reject: %w", err)
	}

	return sendJSONWebhookBytesWithClient(requestCtx, GetStrictHTTPSDirectSSRFProtectedHTTPClient(), webhookURL, secret, deliveryID, payloadBytes)
}

func sendJSONWebhookWithClient(ctx context.Context, client *http.Client, webhookURL string, secret string, deliveryID string, payload any) error {
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}
	return sendJSONWebhookBytesWithClient(ctx, client, webhookURL, secret, deliveryID, payloadBytes)
}

func sendJSONWebhookBytesWithClient(ctx context.Context, client *http.Client, webhookURL string, secret string, deliveryID string, payload []byte) error {
	if client == nil {
		return fmt.Errorf("webhook HTTP client is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if deliveryID != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set(WebhookTimestampHeader, timestamp)
		req.Header.Set(WebhookDeliveryIDHeader, deliveryID)
		if secret != "" {
			req.Header.Set("X-Webhook-Signature", generateVersionedSignature(secret, timestamp, deliveryID, payload))
		}
	} else if secret != "" {
		req.Header.Set("X-Webhook-Signature", generateSignature(secret, payload))
	}

	// Webhook delivery is defined as one POST to the registered endpoint. Go
	// rewrites POST to GET for 301/302/303 redirects and copies custom headers,
	// which could both falsely acknowledge delivery and disclose the signature
	// to another origin. Clone the client so the shared transport remains reusable
	// while redirects are rejected for this request.
	deliveryClient := *client
	deliveryClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := deliveryClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", sanitizedWebhookTransportError{cause: err})
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}
	return nil
}

// SendWebhookNotify 发送 webhook 通知
func SendWebhookNotify(webhookURL string, secret string, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = fmt.Sprintf(content, value)
	}

	// 构建 webhook 负载
	payload := WebhookPayload{
		Type:      data.Type,
		Title:     data.Title,
		Content:   content,
		Values:    data.Values,
		Timestamp: time.Now().Unix(),
	}

	// 序列化负载
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	// 创建 HTTP 请求
	var req *http.Request
	var resp *http.Response

	if system_setting.EnableWorker() {
		// 构建worker请求数据
		workerReq := &WorkerRequest{
			URL:    webhookURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: payloadBytes,
		}

		// 如果有secret，添加签名到headers
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			workerReq.Headers["X-Webhook-Signature"] = signature
			workerReq.Headers["Authorization"] = "Bearer " + secret
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send webhook request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Webhook URL（非Worker模式）
		if err := ValidateSSRFProtectedFetchURL(webhookURL); err != nil {
			return fmt.Errorf("request reject: %v", err)
		}

		req, err = http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %v", err)
		}

		// 设置请求头
		req.Header.Set("Content-Type", "application/json")

		// 如果有 secret，生成签名
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			req.Header.Set("X-Webhook-Signature", signature)
		}

		// 发送请求
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send webhook request: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}
