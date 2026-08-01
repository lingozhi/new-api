package kie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
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

			usage, apiErr := (&Adaptor{wait: noWait}).DoResponse(c, accepted, info)

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

func TestDoResponseHonorsRetryAfterUntilKIESucceeds(t *testing.T) {
	t.Parallel()

	var pollCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := pollCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.Header().Set("Retry-After", "2")
			_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-retry","state":"generating"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":200,"data":{"taskId":"kie-task-retry","state":"success","resultJson":{"resultObject":{"resultUrls":["https://oss.example.com/final.webp"]}}}}`)
	}))
	t.Cleanup(server.Close)

	var mu sync.Mutex
	delays := make([]time.Duration, 0, 2)
	adaptor := &Adaptor{wait: func(_ context.Context, delay time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		delays = append(delays, delay)
		return nil
	}}
	recorder := httptest.NewRecorder()
	c := kieTestContextWithRecorder(context.Background(), recorder)
	info := kieTestRelayInfo(server.URL, relayconstant.RelayModeImagesGenerations, ModelGPTImage2TextToImage)
	accepted := kieHTTPResponse(http.StatusOK, `{"code":200,"data":{"taskId":"kie-task-retry"}}`)
	accepted.Header.Set("Retry-After", "7")

	_, apiErr := adaptor.DoResponse(c, accepted, info)

	require.Nil(t, apiErr)
	assert.Equal(t, int32(2), pollCalls.Load())
	mu.Lock()
	assert.Equal(t, []time.Duration{7 * time.Second, 2 * time.Second}, delays)
	mu.Unlock()
	assert.Contains(t, recorder.Body.String(), `"url":"https://oss.example.com/final.webp"`)
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

	_, apiErr := (&Adaptor{wait: noWait}).DoResponse(c, accepted, info)

	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "upstream generation failed")
	assert.NotErrorIs(t, apiErr, types.ErrProviderTaskPollingRetryable, "terminal provider failure must be eligible for channel failover")
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

	_, apiErr := (&Adaptor{wait: noWait}).DoResponse(c, accepted, info)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, types.ErrProviderTaskPollingRetryable)
}

func TestDefaultKIEPollPolicyUsesThirtySecondRequestsAndFifteenMinuteDeadline(t *testing.T) {
	t.Parallel()

	policy := defaultPollPolicy()

	assert.Equal(t, 30*time.Second, policy.requestTimeout)
	assert.Equal(t, 15*time.Minute, policy.overallTimeout)
}

func noWait(context.Context, time.Duration) error { return nil }

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
