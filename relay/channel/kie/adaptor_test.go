package kie

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	os.Exit(m.Run())
}

func TestConvertImageRequestMapsGPTImage2OperationsWithoutDroppingParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		relayMode     int
		upstreamModel string
		request       dto.ImageRequest
		wantInput     map[string]any
	}{
		{
			name:          "generation",
			relayMode:     relayconstant.RelayModeImagesGenerations,
			upstreamModel: ModelGPTImage2TextToImage,
			request: dto.ImageRequest{
				Model:  "gpt-image-2",
				Prompt: "A lighthouse during a summer storm",
				Extra: map[string]json.RawMessage{
					"aspect_ratio": json.RawMessage(`"16:9"`),
					"resolution":   json.RawMessage(`"4K"`),
				},
			},
			wantInput: map[string]any{
				"prompt":       "A lighthouse during a summer storm",
				"aspect_ratio": "16:9",
				"resolution":   "4K",
			},
		},
		{
			name:          "edit",
			relayMode:     relayconstant.RelayModeImagesEdits,
			upstreamModel: ModelGPTImage2ImageToImage,
			request: dto.ImageRequest{
				Model:  "gpt-image-2",
				Prompt: "Turn the reference into a premium product poster",
				Images: json.RawMessage(`[
					"https://assets.example.com/source-1.png",
					"https://assets.example.com/source-2.png"
				]`),
				Extra: map[string]json.RawMessage{
					"aspect_ratio": json.RawMessage(`"3:4"`),
					"resolution":   json.RawMessage(`"2K"`),
				},
			},
			wantInput: map[string]any{
				"prompt": "Turn the reference into a premium product poster",
				"input_urls": []any{
					"https://assets.example.com/source-1.png",
					"https://assets.example.com/source-2.png",
				},
				"aspect_ratio": "3:4",
				"resolution":   "2K",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := kieTestRelayInfo("https://api.kie.ai", tt.relayMode, tt.upstreamModel)
			c := kieTestContext(context.Background())

			converted, err := (&Adaptor{}).ConvertImageRequest(c, info, tt.request)
			require.NoError(t, err)

			encoded, err := common.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(encoded, &payload))
			assert.Equal(t, tt.upstreamModel, payload["model"])
			assert.Equal(t, tt.wantInput, payload["input"])
			assert.NotContains(t, payload, "callBackUrl", "the gateway owns webhook delivery")

			requestURL, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.kie.ai/api/v1/jobs/createTask", requestURL)

			header := make(http.Header)
			require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
			assert.Equal(t, "Bearer kie-secret", header.Get("Authorization"))
			assert.Equal(t, "application/json", header.Get("Content-Type"))
		})
	}
}

func TestDoResponseResumesAcceptedTaskCheckpointWithoutSubmittingAgain(t *testing.T) {
	t.Parallel()

	for _, submitStatus := range []int{http.StatusOK, http.StatusAccepted} {
		t.Run(http.StatusText(submitStatus), func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			var gotPath atomic.Value
			var gotTaskID atomic.Value
			var gotAuthorization atomic.Value
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				gotPath.Store(r.URL.Path)
				gotTaskID.Store(r.URL.Query().Get("taskId"))
				gotAuthorization.Store(r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{
					"code":200,
					"msg":"success",
					"data":{
						"taskId":"kie-task-123",
						"state":"success",
						"resultJson":"{\"resultUrls\":[\"https://oss.example.com/result.png\"]}"
					}
				}`)
			}))
			t.Cleanup(server.Close)

			recorder := httptest.NewRecorder()
			c := kieTestContextWithRecorder(context.Background(), recorder)
			info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
			accepted := kieHTTPResponse(submitStatus, `{"code":200,"msg":"success","data":{"taskId":"kie-task-123"}}`)

			usage, apiErr := (&Adaptor{}).DoResponse(c, accepted, info)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, int32(1), calls.Load(), "a stored task-id checkpoint must only be polled")
			assert.Equal(t, "/api/v1/jobs/recordInfo", gotPath.Load())
			assert.Equal(t, "kie-task-123", gotTaskID.Load())
			assert.Equal(t, "Bearer kie-secret", gotAuthorization.Load())

			var response dto.ImageResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Len(t, response.Data, 1)
			assert.Equal(t, "https://oss.example.com/result.png", response.Data[0].Url)
		})
	}
}

func TestDoResponsePollsOnceAndReturnsTypedPendingRetry(t *testing.T) {
	t.Parallel()

	var pollCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pollCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "90")
		_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-retry","state":"generating"}}`)
	}))
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	c := kieTestContextWithRecorder(context.Background(), recorder)
	info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
	accepted := kieHTTPResponse(http.StatusOK, `{"code":200,"data":{"taskId":"kie-task-retry"}}`)

	_, apiErr := (&Adaptor{}).DoResponse(c, accepted, info)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, types.ErrProviderTaskPollingRetryable)
	assert.Equal(t, http.StatusAccepted, apiErr.StatusCode, "normal pending is scheduling metadata, not a provider failure")
	assert.Zero(t, apiErr.UpstreamStatusCode)
	retryAfter, ok := types.ProviderTaskPollingRetryAfter(apiErr)
	require.True(t, ok)
	assert.Equal(t, time.Minute, retryAfter)
	assert.Equal(t, int32(1), pollCalls.Load(), "one worker turn must issue at most one poll")
	assert.Empty(t, recorder.Body.String())
}

func TestDoResponseReturnsKIEFailureAsTerminal(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-failed","state":"fail","failCode":"501","failMsg":"upstream generation failed"}}`)
	}))
	t.Cleanup(server.Close)

	c := kieTestContext(context.Background())
	info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
	accepted := kieHTTPResponse(http.StatusOK, `{"code":200,"data":{"taskId":"kie-task-failed"}}`)

	_, apiErr := (&Adaptor{}).DoResponse(c, accepted, info)

	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "upstream generation failed")
	assert.NotErrorIs(t, apiErr, types.ErrProviderTaskPollingRetryable)
	assert.ErrorIs(t, apiErr, types.ErrProviderTaskUnsafeToResubmit, "accepted terminal failure may already have been billed")
}

func TestDoResponseMarksInterruptedKIEPollingRecoverable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `temporarily unavailable`)
	}))
	t.Cleanup(server.Close)

	c := kieTestContext(context.Background())
	info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
	accepted := kieHTTPResponse(http.StatusOK, `{"code":200,"data":{"taskId":"kie-task-recover"}}`)

	_, apiErr := (&Adaptor{}).DoResponse(c, accepted, info)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, types.ErrProviderTaskPollingRetryable)
}

func TestDefaultKIEPollPolicyUsesThirtySecondRequestWithoutLocalOverallDeadline(t *testing.T) {
	t.Parallel()

	policy := defaultPollPolicy()

	assert.Equal(t, 30*time.Second, policy.requestTimeout)
}

func TestRetryAfterDelayIsBoundedAndInvalidValuesUseDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "missing", want: 2 * time.Second},
		{name: "minimum", value: "1", want: time.Second},
		{name: "clamped maximum", value: "999", want: time.Minute},
		{name: "zero", value: "0", want: 2 * time.Second},
		{name: "negative", value: "-1", want: 2 * time.Second},
		{name: "integer overflow", value: "999999999999999999999999", want: 2 * time.Second},
		{name: "invalid", value: "later", want: 2 * time.Second},
		{name: "past date", value: now.Add(-time.Minute).Format(http.TimeFormat), want: 2 * time.Second},
		{name: "future date clamped", value: now.Add(5 * time.Minute).Format(http.TimeFormat), want: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(http.Header)
			if tt.value != "" {
				header.Set("Retry-After", tt.value)
			}
			assert.Equal(t, tt.want, retryAfterDelay(header, now))
		})
	}
}

func TestPollAppliesResolvedHeaderOverridesWithoutLeakingRawChannelKey(t *testing.T) {
	t.Parallel()

	requestHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-auth","state":"success","resultJson":{"resultUrls":["https://oss.example.com/auth.png"]}}}`)
	}))
	t.Cleanup(server.Close)

	info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
	info.HeadersOverride = map[string]any{
		"Authorization": "Bearer advanced-custom-secret",
		"X-KIE-Tenant":  "tenant-a",
	}
	c := kieTestContextWithRecorder(context.Background(), httptest.NewRecorder())
	accepted := kieHTTPResponse(http.StatusAccepted, `{"code":200,"data":{"taskId":"kie-task-auth"}}`)

	_, apiErr := (&Adaptor{}).DoResponse(c, accepted, info)
	require.Nil(t, apiErr)

	headers := <-requestHeaders
	assert.Equal(t, "Bearer advanced-custom-secret", headers.Get("Authorization"))
	assert.Equal(t, "tenant-a", headers.Get("X-KIE-Tenant"))
	assert.NotContains(t, strings.Join(headers.Values("Authorization"), " "), "kie-secret")
}

func TestSubmitBusinessEnvelopePreservesDefinitiveAndRetryStatuses(t *testing.T) {
	t.Parallel()

	tests := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError}
	for _, code := range tests {
		t.Run(http.StatusText(code), func(t *testing.T) {
			accepted := kieHTTPResponse(http.StatusOK, `{"code":`+strconv.Itoa(code)+`,"msg":"classified failure","data":null}`)
			_, apiErr := (&Adaptor{}).DoResponse(kieTestContext(context.Background()), accepted, kieTestRelayInfo("https://api.kie.ai", relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage))
			require.NotNil(t, apiErr)
			assert.Equal(t, code, apiErr.StatusCode)
			assert.NotErrorIs(t, apiErr, types.ErrProviderTaskUnsafeToResubmit, "submit rejection has no accepted task id")
		})
	}
}

func TestPollBusinessEnvelopeClassifiesRetryAndCredentialFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		code           int
		wantRetry      bool
		wantUnsafe     bool
		wantRetryAfter time.Duration
	}{
		{name: "rate limited", code: http.StatusTooManyRequests, wantRetry: true, wantRetryAfter: 3 * time.Second},
		{name: "upstream unavailable", code: http.StatusServiceUnavailable, wantRetry: true, wantRetryAfter: 3 * time.Second},
		{name: "credential failure", code: http.StatusUnauthorized, wantUnsafe: true},
		{name: "invalid accepted task", code: http.StatusUnprocessableEntity, wantUnsafe: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"code":`+strconv.Itoa(tt.code)+`,"msg":"poll classified failure","data":null}`)
			}))
			t.Cleanup(server.Close)

			accepted := kieHTTPResponse(http.StatusAccepted, `{"code":200,"data":{"taskId":"kie-task-envelope"}}`)
			_, apiErr := (&Adaptor{}).DoResponse(kieTestContext(context.Background()), accepted, kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage))
			require.NotNil(t, apiErr)
			assert.Equal(t, tt.code, apiErr.StatusCode)
			assert.Equal(t, tt.wantRetry, errors.Is(apiErr, types.ErrProviderTaskPollingRetryable))
			assert.Equal(t, tt.wantUnsafe, errors.Is(apiErr, types.ErrProviderTaskUnsafeToResubmit))
			if tt.wantRetry {
				delay, ok := types.ProviderTaskPollingRetryAfter(apiErr)
				require.True(t, ok)
				assert.Equal(t, tt.wantRetryAfter, delay)
			}
		})
	}
}

func TestDecodeCreateTaskIDRejectsUnsafeOrUnboundedIdentifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		taskID string
		ok     bool
	}{
		{name: "uuid style", taskID: "task_01J.ABC:def-123", ok: true},
		{name: "max length", taskID: strings.Repeat("a", 256), ok: true},
		{name: "too long", taskID: strings.Repeat("a", 257)},
		{name: "slash", taskID: "task/123"},
		{name: "space", taskID: "task 123"},
		{name: "unicode", taskID: "task-图片"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"code":200,"data":{"taskId":"` + tt.taskID + `"}}`)
			got, apiErr := decodeCreateTaskID(body)
			if tt.ok {
				require.Nil(t, apiErr)
				assert.Equal(t, tt.taskID, got)
				return
			}
			require.NotNil(t, apiErr)
			assert.Empty(t, got)
		})
	}
}

func TestDoResponseRejectsMultipleResultURLs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-multi","state":"success","resultJson":{"resultUrls":["https://oss.example.com/one.png","https://oss.example.com/two.png"]}}}`)
	}))
	t.Cleanup(server.Close)

	accepted := kieHTTPResponse(http.StatusAccepted, `{"code":200,"data":{"taskId":"kie-task-multi"}}`)
	_, apiErr := (&Adaptor{}).DoResponse(kieTestContext(context.Background()), accepted, kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage))

	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "exactly one image URL")
	assert.ErrorIs(t, apiErr, types.ErrProviderTaskUnsafeToResubmit)
}

func kieTestRelayInfo(baseURL string, relayMode int, upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:            relayMode,
		RelayFormat:          types.RelayFormatOpenAIImage,
		OriginModelName:      "gpt-image-2",
		RequestURLPath:       "/v1/jobs",
		ImageRoutingProtocol: dto.ImageRoutingProtocolKIEJobs,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			ApiKey:            "kie-secret",
			UpstreamModelName: upstreamModel,
		},
	}
}

func kieTestContext(ctx context.Context) *gin.Context {
	return kieTestContextWithRecorder(ctx, httptest.NewRecorder())
}

func kieTestContextWithRecorder(ctx context.Context, recorder *httptest.ResponseRecorder) *gin.Context {
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/jobs", nil).WithContext(ctx)
	return c
}

func kieHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request: &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/v1/jobs/createTask"},
		},
	}
}
