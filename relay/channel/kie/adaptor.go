package kie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	defaultPollInterval     = 2 * time.Second
	minPollInterval         = time.Second
	maxPollInterval         = time.Minute
	defaultPollRequestLimit = 30 * time.Second
	maxKIEResponseBodySize  = 4 << 20
	maxKIETaskIDLength      = 256
)

type pollPolicy struct {
	requestTimeout time.Duration
}

func defaultPollPolicy() pollPolicy {
	return pollPolicy{
		requestTimeout: defaultPollRequestLimit,
	}
}

type Adaptor struct{}

type createTaskRequest struct {
	Model string          `json:"model"`
	Input createTaskInput `json:"input"`
}

type createTaskInput struct {
	Prompt      string   `json:"prompt"`
	InputURLs   []string `json:"input_urls,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
}

type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type createTaskData struct {
	TaskID string `json:"taskId"`
}

type taskRecord struct {
	TaskID     string          `json:"taskId"`
	State      string          `json:"state"`
	ResultJSON json.RawMessage `json:"resultJson"`
	FailCode   string          `json:"failCode"`
	FailMsg    string          `json:"failMsg"`
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (r *cancelOnCloseReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil {
		r.once.Do(r.cancel)
	}
	return n, err
}

func (r *cancelOnCloseReadCloser) Close() error {
	r.once.Do(r.cancel)
	return r.ReadCloser.Close()
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("kie adaptor: relay info is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(info.ChannelBaseUrl), "/")
	if baseURL == "" {
		return "", errors.New("kie adaptor: channel base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("kie adaptor: invalid channel base URL %q", baseURL)
	}
	return baseURL + createTaskPath, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("kie adaptor: relay info is required")
	}
	if strings.TrimSpace(info.ApiKey) == "" {
		return errors.New("kie adaptor: API key is required")
	}
	if c != nil && c.Request != nil {
		channel.SetupApiRequestHeader(info, c, header)
	}
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	return nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil {
		return nil, errors.New("kie adaptor: relay info is required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("kie adaptor: prompt is required")
	}
	if utf8.RuneCountInString(request.Prompt) > dto.MaxUnifiedImagePromptLength {
		return nil, fmt.Errorf("kie adaptor: prompt is too long (max %d characters)", dto.MaxUnifiedImagePromptLength)
	}
	if request.N != nil && *request.N != 1 {
		return nil, errors.New("kie adaptor: gpt-image-2 supports only n=1")
	}
	if len(strings.TrimSpace(string(request.Mask))) > 0 && common.GetJsonType(request.Mask) != "null" {
		return nil, errors.New("kie adaptor: gpt-image-2 jobs do not support masks")
	}

	input := createTaskInput{Prompt: request.Prompt}
	var err error
	input.AspectRatio, err = optionalString(request.Extra, "aspect_ratio")
	if err != nil {
		return nil, err
	}
	input.Resolution, err = optionalString(request.Extra, "resolution")
	if err != nil {
		return nil, err
	}

	model := ""
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
		model = ModelGPTImage2TextToImage
	case relayconstant.RelayModeImagesEdits:
		model = ModelGPTImage2ImageToImage
		input.InputURLs, err = request.ImageInputURLs()
		if err != nil {
			return nil, fmt.Errorf("kie adaptor: invalid input_urls: %w", err)
		}
		if len(input.InputURLs) == 0 {
			return nil, errors.New("kie adaptor: input_urls are required for image edits")
		}
	default:
		return nil, fmt.Errorf("kie adaptor: unsupported image relay mode %d", info.RelayMode)
	}
	if !isGPTImage2Model(info.OriginModelName) && !isGPTImage2Model(info.UpstreamModelName) {
		return nil, fmt.Errorf("kie adaptor: unsupported model %q", info.UpstreamModelName)
	}

	return createTaskRequest{Model: model, Input: input}, nil
}

func optionalString(extra map[string]json.RawMessage, name string) (string, error) {
	raw, ok := extra[name]
	if !ok || len(strings.TrimSpace(string(raw))) == 0 || common.GetJsonType(raw) == "null" {
		return "", nil
	}
	if common.GetJsonType(raw) != "string" {
		return "", fmt.Errorf("kie adaptor: %s must be a string", name)
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("kie adaptor: decode %s: %w", name, err)
	}
	return value, nil
}

func isGPTImage2Model(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-image-2", ModelGPTImage2TextToImage, ModelGPTImage2ImageToImage:
		return true
	default:
		return false
	}
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("kie adaptor: request context is required")
	}
	policy := defaultPollPolicy()
	requestCtx, cancel := context.WithTimeout(c.Request.Context(), policy.requestTimeout)
	originalRequest := c.Request
	c.Request = c.Request.Clone(requestCtx)
	response, err := channel.DoApiRequest(a, c, info, requestBody)
	c.Request = originalRequest
	if err != nil {
		cancel()
		return nil, err
	}
	if response == nil || response.Body == nil {
		cancel()
		return nil, errors.New("kie adaptor: empty submit response")
	}
	response.Body = &cancelOnCloseReadCloser{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("kie adaptor: empty submit response"), types.ErrorCodeBadResponse)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return nil, providerStatusAPIError("submit", response.StatusCode, false)
	}

	body, err := readBoundedBody(response.Body)
	if err != nil {
		return nil, acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: read submit response: %w", err),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusBadGateway,
			0,
		)
	}
	taskID, apiErr := decodeCreateTaskID(body)
	if apiErr != nil {
		return nil, apiErr
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	policy := defaultPollPolicy()
	record, header, pollErr := pollTask(ctx, policy.requestTimeout, c, info, taskID)
	if pollErr != nil {
		return nil, pollErr
	}
	if record.TaskID != "" {
		if err := validateTaskID(record.TaskID); err != nil {
			return nil, acceptedTaskFailureAPIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, 0)
		}
		if record.TaskID != taskID {
			return nil, acceptedTaskFailureAPIError(
				fmt.Errorf("kie adaptor: poll taskId mismatch"),
				types.ErrorCodeBadResponseBody,
				http.StatusBadGateway,
				0,
			)
		}
	}

	switch strings.ToLower(strings.TrimSpace(record.State)) {
	case "success":
		urls, err := normalizeResultURLs(record.ResultJSON)
		if err != nil {
			return nil, acceptedTaskFailureAPIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, 0)
		}
		if len(urls) != 1 {
			return nil, acceptedTaskFailureAPIError(
				fmt.Errorf("kie adaptor: completed task must return exactly one image URL (got %d)", len(urls)),
				types.ErrorCodeBadResponseBody,
				http.StatusBadGateway,
				0,
			)
		}
		if err := writeImageResponse(c, urls); err != nil {
			return nil, acceptedTaskFailureAPIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, 0)
		}
		return &dto.Usage{}, nil
	case "fail", "failed":
		message := safeKIEProviderMessage(record.FailMsg)
		if message == "" {
			message = "KIE image generation failed"
		}
		if code := strings.TrimSpace(record.FailCode); code != "" {
			message = fmt.Sprintf("%s (code %s)", message, safeKIEProviderMessage(code))
		}
		return nil, acceptedTaskFailureAPIError(errors.New(message), types.ErrorCodeBadResponse, http.StatusBadGateway, 0)
	case "waiting", "queuing", "queued", "generating", "pending":
		delay := retryAfterDelay(header, time.Now())
		return nil, retryablePollAPIError("task is still processing", nil, delay, http.StatusAccepted, 0)
	default:
		return nil, acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: unsupported task state %q", safeKIEProviderMessage(record.State)),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			0,
		)
	}
}

func pollTask(ctx context.Context, requestTimeout time.Duration, c *gin.Context, info *relaycommon.RelayInfo, taskID string) (taskRecord, http.Header, *types.NewAPIError) {
	if info == nil {
		return taskRecord{}, nil, acceptedTaskFailureAPIError(
			errors.New("kie adaptor: relay info is required for polling"),
			types.ErrorCodeInvalidRequest,
			http.StatusInternalServerError,
			0,
		)
	}
	if err := validateTaskID(taskID); err != nil {
		return taskRecord{}, nil, acceptedTaskFailureAPIError(err, types.ErrorCodeInvalidRequest, http.StatusInternalServerError, 0)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(info.ChannelBaseUrl), "/")
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return taskRecord{}, nil, acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: invalid channel base URL %q", baseURL),
			types.ErrorCodeInvalidRequest,
			http.StatusInternalServerError,
			0,
		)
	}
	pollURL := baseURL + taskRecordPath + "?taskId=" + url.QueryEscape(taskID)
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return taskRecord{}, nil, retryablePollAPIError("create polling client", err, defaultPollInterval, http.StatusBadGateway, 0)
	}

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, pollURL, nil)
	if err != nil {
		return taskRecord{}, nil, acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: create poll request: %w", err),
			types.ErrorCodeInvalidRequest,
			http.StatusInternalServerError,
			0,
		)
	}
	request.Header.Set("Authorization", "Bearer "+info.ApiKey)
	request.Header.Set("Accept", "application/json")
	headerOverride, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return taskRecord{}, nil, acceptedTaskFailureAPIError(
			err,
			types.ErrorCodeChannelHeaderOverrideInvalid,
			http.StatusInternalServerError,
			0,
		)
	}
	channel.ApplyHeaderOverrideToRequest(request, headerOverride)

	response, err := client.Do(request)
	if err != nil {
		return taskRecord{}, nil, retryablePollAPIError("poll request failed", err, defaultPollInterval, http.StatusBadGateway, 0)
	}
	body, err := readBoundedBody(response.Body)
	if err != nil {
		return taskRecord{}, nil, retryablePollAPIError("read poll response", err, retryAfterDelay(response.Header, time.Now()), http.StatusBadGateway, 0)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if isRetryablePollStatus(response.StatusCode) {
			return taskRecord{}, response.Header.Clone(), retryablePollAPIError(
				fmt.Sprintf("poll returned HTTP %d", response.StatusCode),
				nil,
				retryAfterDelay(response.Header, time.Now()),
				response.StatusCode,
				response.StatusCode,
			)
		}
		return taskRecord{}, response.Header.Clone(), acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: poll returned HTTP %d", response.StatusCode),
			providerStatusErrorCode(response.StatusCode),
			response.StatusCode,
			response.StatusCode,
		)
	}

	record, code, message, err := decodeTaskRecord(body)
	if err != nil {
		return taskRecord{}, response.Header.Clone(), acceptedTaskFailureAPIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, 0)
	}
	if code != 0 && code != http.StatusOK {
		status := kieEnvelopeStatus(code)
		if isRetryablePollStatus(status) {
			return taskRecord{}, response.Header.Clone(), retryablePollAPIError(
				fmt.Sprintf("poll returned code %d: %s", code, safeKIEProviderMessage(message)),
				nil,
				retryAfterDelay(response.Header, time.Now()),
				status,
				status,
			)
		}
		return taskRecord{}, response.Header.Clone(), acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: poll returned code %d: %s", code, safeKIEProviderMessage(message)),
			providerStatusErrorCode(status),
			status,
			status,
		)
	}
	return record, response.Header.Clone(), nil
}

func readBoundedBody(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxKIEResponseBodySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxKIEResponseBodySize {
		return nil, fmt.Errorf("response exceeds %d bytes", maxKIEResponseBodySize)
	}
	return data, nil
}

func decodeCreateTaskID(body []byte) (string, *types.NewAPIError) {
	var envelope apiResponse
	if err := common.Unmarshal(body, &envelope); err != nil {
		return "", acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: decode submit response: %w", err),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			0,
		)
	}
	if envelope.Code != 0 && envelope.Code != http.StatusOK {
		status := kieEnvelopeStatus(envelope.Code)
		apiErr := types.NewErrorWithStatusCode(
			fmt.Errorf("kie adaptor: submit returned code %d: %s", envelope.Code, safeKIEProviderMessage(envelope.Msg)),
			providerStatusErrorCode(status),
			status,
		)
		apiErr.UpstreamStatusCode = status
		return "", apiErr
	}
	data := envelope.Data
	if len(data) == 0 || common.GetJsonType(data) == "null" {
		data = append(json.RawMessage(nil), body...)
	}
	var accepted createTaskData
	if err := common.Unmarshal(data, &accepted); err != nil {
		return "", acceptedTaskFailureAPIError(
			fmt.Errorf("kie adaptor: decode submit task data: %w", err),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			0,
		)
	}
	accepted.TaskID = strings.TrimSpace(accepted.TaskID)
	if err := validateTaskID(accepted.TaskID); err != nil {
		return "", acceptedTaskFailureAPIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, 0)
	}
	return accepted.TaskID, nil
}

func validateTaskID(taskID string) error {
	if taskID == "" {
		return errors.New("kie adaptor: submit response is missing taskId")
	}
	if len(taskID) > maxKIETaskIDLength {
		return fmt.Errorf("kie adaptor: taskId exceeds %d characters", maxKIETaskIDLength)
	}
	for _, char := range taskID {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return errors.New("kie adaptor: taskId contains unsupported characters")
	}
	return nil
}

func decodeTaskRecord(body []byte) (taskRecord, int, string, error) {
	var envelope apiResponse
	if err := common.Unmarshal(body, &envelope); err != nil {
		return taskRecord{}, 0, "", fmt.Errorf("kie adaptor: decode poll response: %w", err)
	}
	data := envelope.Data
	if len(data) == 0 || common.GetJsonType(data) == "null" {
		data = append(json.RawMessage(nil), body...)
	}
	var record taskRecord
	if err := common.Unmarshal(data, &record); err != nil {
		return taskRecord{}, envelope.Code, envelope.Msg, fmt.Errorf("kie adaptor: decode task record: %w", err)
	}
	return record, envelope.Code, envelope.Msg, nil
}

func normalizeResultURLs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return nil, nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("kie adaptor: decode resultJson: %w", err)
	}
	for depth := 0; depth < 3; depth++ {
		text, ok := value.(string)
		if !ok {
			break
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		if err := common.Unmarshal([]byte(text), &value); err != nil {
			return nil, fmt.Errorf("kie adaptor: decode nested resultJson: %w", err)
		}
	}
	return resultURLsFromValue(value), nil
}

func resultURLsFromValue(value any) []string {
	record, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if urls := stringURLs(record["resultUrls"]); len(urls) > 0 {
		return urls
	}
	for _, key := range []string{"resultObject", "result", "output", "data"} {
		if urls := resultURLsFromValue(record[key]); len(urls) > 0 {
			return urls
		}
	}
	return nil
}

func stringURLs(value any) []string {
	if encoded, ok := value.(string); ok {
		var decoded any
		if common.Unmarshal([]byte(strings.TrimSpace(encoded)), &decoded) != nil {
			return nil
		}
		value = decoded
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		rawURL, ok := item.(string)
		if !ok {
			continue
		}
		rawURL = strings.TrimSpace(rawURL)
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			continue
		}
		urls = append(urls, rawURL)
	}
	return urls
}

func writeImageResponse(c *gin.Context, urls []string) error {
	if c == nil || c.Writer == nil {
		return errors.New("kie adaptor: response writer is required")
	}
	data := make([]dto.ImageData, 0, len(urls))
	for _, resultURL := range urls {
		data = append(data, dto.ImageData{Url: resultURL})
	}
	encoded, err := common.Marshal(dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    data,
	})
	if err != nil {
		return fmt.Errorf("kie adaptor: encode image response: %w", err)
	}
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)
	_, err = c.Writer.Write(encoded)
	return err
}

func retryAfterDelay(header http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return defaultPollInterval
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return defaultPollInterval
		}
		if seconds > int64(maxPollInterval/time.Second) {
			return maxPollInterval
		}
		return clampPollInterval(time.Duration(seconds) * time.Second)
	}
	if deadline, err := http.ParseTime(value); err == nil {
		delay := deadline.Sub(now)
		if delay > 0 {
			return clampPollInterval(delay)
		}
		return defaultPollInterval
	}
	return defaultPollInterval
}

func clampPollInterval(delay time.Duration) time.Duration {
	if delay < minPollInterval {
		return minPollInterval
	}
	if delay > maxPollInterval {
		return maxPollInterval
	}
	return delay
}

func isRetryablePollStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusConflict ||
		status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func retryablePollAPIError(message string, cause error, retryAfter time.Duration, statusCode int, upstreamStatusCode int) *types.NewAPIError {
	err := errors.New("kie adaptor: " + message)
	if cause != nil {
		err = fmt.Errorf("kie adaptor: %s: %w", message, cause)
	}
	retryErr := types.NewProviderTaskPollingRetryError(err, clampPollInterval(retryAfter))
	apiErr := types.NewErrorWithStatusCode(retryErr, types.ErrorCodeBadResponse, statusCode)
	apiErr.UpstreamStatusCode = upstreamStatusCode
	return apiErr
}

func acceptedTaskFailureAPIError(cause error, errorCode types.ErrorCode, statusCode int, upstreamStatusCode int) *types.NewAPIError {
	apiErr := types.NewErrorWithStatusCode(
		types.NewProviderTaskUnsafeToResubmitError(cause),
		errorCode,
		statusCode,
	)
	apiErr.UpstreamStatusCode = upstreamStatusCode
	return apiErr
}

func providerStatusAPIError(phase string, statusCode int, accepted bool) *types.NewAPIError {
	cause := fmt.Errorf("kie adaptor: %s returned HTTP %d", phase, statusCode)
	if accepted {
		return acceptedTaskFailureAPIError(cause, providerStatusErrorCode(statusCode), statusCode, statusCode)
	}
	apiErr := types.NewErrorWithStatusCode(cause, providerStatusErrorCode(statusCode), statusCode)
	apiErr.UpstreamStatusCode = statusCode
	return apiErr
}

func providerStatusErrorCode(statusCode int) types.ErrorCode {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return types.ErrorCodeChannelInvalidKey
	}
	return types.ErrorCodeBadResponseStatusCode
}

func kieEnvelopeStatus(code int) int {
	if code >= http.StatusBadRequest && code <= 599 {
		return code
	}
	return http.StatusBadGateway
}

func safeKIEProviderMessage(message string) string {
	message = common.MaskSensitiveInfo(strings.TrimSpace(message))
	const maxMessageLength = 512
	if len(message) > maxMessageLength {
		return message[:maxMessageLength] + "..."
	}
	return message
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("kie adaptor: chat requests are not supported")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("kie adaptor: rerank requests are not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("kie adaptor: embedding requests are not supported")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("kie adaptor: audio requests are not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("kie adaptor: responses requests are not supported")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("kie adaptor: Claude requests are not supported")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("kie adaptor: Gemini requests are not supported")
}
