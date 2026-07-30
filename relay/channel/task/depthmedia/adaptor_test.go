package depthmedia

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRelayInfo(baseURL, apiKey, action string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: baseURL,
			ApiKey:         apiKey,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action:       action,
			PublicTaskID: "task_public",
		},
	}
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		quality   string
		scale     int
		want      string
	}{
		{name: "background fast", operation: "remove_background", quality: "fast", want: ModelBackgroundFast},
		{name: "background quality", operation: "remove_background", quality: "quality", want: ModelBackgroundQuality},
		{name: "background matting", operation: "remove_background", quality: "matting", want: ModelBackgroundMatting},
		{name: "upscale fast 2x", operation: "upscale", quality: "fast", scale: 2, want: ModelUpscaleFast2X},
		{name: "upscale fast 4x", operation: "upscale", quality: "fast", scale: 4, want: ModelUpscaleFast4X},
		{name: "upscale fidelity", operation: "upscale", quality: "fidelity", scale: 4, want: ModelUpscaleFidelity4X},
		{name: "upscale sharp", operation: "upscale", quality: "sharp", scale: 4, want: ModelUpscaleSharp4X},
		{name: "subtitle removal", operation: "remove_subtitles", quality: "quality", want: ModelSubtitleRemove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modelName, err := ResolveModel(tt.operation, tt.quality, tt.scale)
			require.NoError(t, err)
			assert.Equal(t, tt.want, modelName)
		})
	}

	_, err := ResolveModel("upscale", "fidelity", 2)
	require.Error(t, err)
	_, err = ResolveModel("remove_subtitles", "quality", 2)
	require.Error(t, err)
}

func TestTaskAdaptorBuildsMediaRequestWithoutGatewayWebhookFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: ModelUpscaleFidelity4X,
		Image: "https://cdn.example.com/input.png",
		Metadata: map[string]any{
			"source_url":     "https://cdn.example.com/input.png",
			"operation":      "upscale",
			"quality":        "fidelity",
			"scale":          4,
			"format":         "webp",
			"webhook_url":    "https://client.example.com/hook",
			"webhook_secret": "do-not-forward",
		},
		WebhookURL:    "https://client.example.com/hook",
		WebhookSecret: "do-not-forward",
	})
	info := newTestRelayInfo("https://modal.example.com", "upstream-secret", ActionMedia)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "https://cdn.example.com/input.png", payload["source_url"])
	assert.Equal(t, "upscale", payload["operation"])
	assert.Equal(t, "fidelity", payload["quality"])
	assert.Equal(t, float64(4), payload["scale"])
	assert.Equal(t, "webp", payload["format"])
	assert.NotContains(t, payload, "webhook_url")
	assert.NotContains(t, payload, "webhook_secret")

	request, err := http.NewRequest(http.MethodPost, "https://modal.example.com/v1/media/jobs", strings.NewReader("{}"))
	require.NoError(t, err)
	require.NoError(t, adaptor.BuildRequestHeader(c, request, info))
	assert.Equal(t, "Bearer upstream-secret", request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
}

func TestTaskAdaptorBuildsDepthRequestWithUnifiedOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: ModelDepthVideo,
		Image: "https://cdn.example.com/input.mp4",
	})
	info := newTestRelayInfo("https://modal.example.com", "upstream-secret", ActionDepth)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "https://cdn.example.com/input.mp4", payload["source_url"])
	assert.Equal(t, "depth", payload["operation"])
}

func TestTaskAdaptorBuildsSubtitleRemovalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/video/generations",
		strings.NewReader(`{
			"model":"subtitle-remove",
			"image":"https://cdn.example.com/captioned.mp4",
			"metadata":{
				"operation":" Remove_Subtitles ",
				"quality":" Quality ",
				"format":"MP4",
				"subtitle_area":"Bottom"
			}
		}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	info := newTestRelayInfo("https://modal.example.com", "upstream-secret", "")
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "https://cdn.example.com/captioned.mp4", payload["source_url"])
	assert.Equal(t, "remove_subtitles", payload["operation"])
	assert.Equal(t, "quality", payload["quality"])
	assert.Equal(t, "mp4", payload["format"])
	assert.Equal(t, "bottom", payload["subtitle_area"])
}

func TestTaskAdaptorValidatesDepthAndMediaRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		body       string
		wantAction string
		wantError  bool
	}{
		{
			name:       "depth",
			body:       `{"model":"depth-anything-v2-small-video","image":"https://cdn.example.com/input.mp4"}`,
			wantAction: ActionDepth,
		},
		{
			name:       "media",
			body:       `{"model":"background-remove-fast","image":"https://cdn.example.com/input.png","metadata":{"operation":"remove_background","quality":"fast"}}`,
			wantAction: ActionMedia,
		},
		{
			name:       "subtitle removal",
			body:       `{"model":"subtitle-remove","image":"https://cdn.example.com/input.mp4","metadata":{"operation":"remove_subtitles","quality":"quality","format":"mp4","subtitle_area":"full"}}`,
			wantAction: ActionMedia,
		},
		{
			name:      "subtitle removal rejects scale",
			body:      `{"model":"subtitle-remove","image":"https://cdn.example.com/input.mp4","metadata":{"operation":"remove_subtitles","quality":"quality","format":"mp4","subtitle_area":"bottom","scale":2}}`,
			wantError: true,
		},
		{
			name:      "missing source",
			body:      `{"model":"depth-anything-v2-small-video"}`,
			wantError: true,
		},
		{
			name:      "invalid source",
			body:      `{"model":"depth-anything-v2-small-video","image":"file:///tmp/input.mp4"}`,
			wantError: true,
		},
		{
			name:      "mismatched profile",
			body:      `{"model":"background-remove-quality","image":"https://cdn.example.com/input.png","metadata":{"operation":"remove_background","quality":"fast"}}`,
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			info := newTestRelayInfo("https://modal.example.com", "key", "")
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			if tt.wantError {
				require.NotNil(t, taskErr)
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, tt.wantAction, info.Action)
			_, exists := c.Get("task_request")
			assert.True(t, exists)
		})
	}
}

func TestTaskAdaptorBuildsUnifiedJobURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := newTestRelayInfo("https://modal.example.com/", "key", ActionDepth)
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://modal.example.com/v1/jobs", requestURL)

	info.Action = ActionMedia
	requestURL, err = adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://modal.example.com/v1/jobs", requestURL)

	info.Action = "unknown"
	_, err = adaptor.BuildRequestURL(info)
	require.Error(t, err)
}

func TestTaskAdaptorReturnsPublicTaskIDForAcceptedSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := newTestRelayInfo("https://modal.example.com", "key", ActionDepth)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"id":"job_upstream","status":"queued","progress":0}`)),
	}

	upstreamID, taskData, taskErr := adaptor.DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "job_upstream", upstreamID)
	assert.JSONEq(t, `{"id":"job_upstream","status":"queued","progress":0}`, string(taskData))
	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"id":"task_public"`)
}

func TestTaskAdaptorFetchesUnifiedJobs(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		assert.Equal(t, "Bearer upstream-key", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job_1","status":"queued","progress":0}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	for _, action := range []string{ActionDepth, ActionMedia} {
		response, err := adaptor.FetchTask(server.URL, "upstream-key", map[string]any{
			"task_id": "job_1",
			"action":  action,
		}, "")
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
	}
	assert.Equal(t, []string{"/v1/jobs/job_1", "/v1/jobs/job_1"}, paths)

	_, err := adaptor.FetchTask(server.URL, "upstream-key", map[string]any{"action": ActionDepth}, "")
	require.Error(t, err)
	_, err = adaptor.FetchTask(server.URL, "upstream-key", map[string]any{"task_id": "job_1", "action": "unknown"}, "")
	require.Error(t, err)
}

func TestTaskAdaptorMetadataAndIdentity(t *testing.T) {
	adaptor := &TaskAdaptor{}
	assert.Equal(t, "depthmedia", adaptor.GetChannelName())
	assert.ElementsMatch(t, supportedModels, adaptor.GetModelList())

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newTestRelayInfo("https://modal.example.com", "key", ActionDepth)
	adaptor.Init(info)
	_, err := adaptor.BuildRequestBody(c, info)
	require.Error(t, err)

	c.Set("task_request", "wrong type")
	_, err = adaptor.BuildRequestBody(c, info)
	require.Error(t, err)

	response := &http.Response{Body: io.NopCloser(bytes.NewBufferString(`{"status":"queued"}`))}
	_, _, taskErr := adaptor.DoResponse(c, response, info)
	require.NotNil(t, taskErr)
}

func TestTaskAdaptorParsesLifecycle(t *testing.T) {
	adaptor := &TaskAdaptor{}

	queued, err := adaptor.ParseTaskResult([]byte(`{"id":"job_1","status":"queued","progress":0}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, queued.Status)
	assert.Equal(t, "0%", queued.Progress)

	completed, err := adaptor.ParseTaskResult([]byte(`{"id":"job_1","status":"completed","progress":100,"result_url":"https://cdn.example.com/result.webp"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, completed.Status)
	assert.Equal(t, "https://cdn.example.com/result.webp", completed.Url)

	failed, err := adaptor.ParseTaskResult([]byte(`{"id":"job_1","status":"failed","error":"worker failed"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failed.Status)
	assert.Equal(t, "worker failed", failed.Reason)

	_, err = adaptor.ParseTaskResult([]byte(`{"id":"job_1","status":"mystery"}`))
	require.Error(t, err)
	_, err = adaptor.ParseTaskResult([]byte(`not-json`))
	require.Error(t, err)
}

func TestTaskAdaptorEstimatesMaximumDepthVideoDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &TaskAdaptor{}

	depthInfo := newTestRelayInfo("https://modal.example.com", "key", ActionDepth)
	depthInfo.OriginModelName = ModelDepthVideo
	assert.Equal(t, map[string]float64{"seconds": 600}, adaptor.EstimateBilling(c, depthInfo))

	imageInfo := newTestRelayInfo("https://modal.example.com", "key", ActionMedia)
	imageInfo.OriginModelName = ModelUpscaleFast2X
	assert.Nil(t, adaptor.EstimateBilling(c, imageInfo))

	subtitleInfo := newTestRelayInfo("https://modal.example.com", "key", ActionMedia)
	subtitleInfo.OriginModelName = ModelSubtitleRemove
	assert.Equal(t, map[string]float64{"seconds": 600}, adaptor.EstimateBilling(c, subtitleInfo))
}

func TestTaskAdaptorReconcilesDepthVideoToActualDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Action: ActionDepth,
		Data: []byte(
			`{"id":"job_1","status":"completed","progress":100,"fps":30,"frames":299}`,
		),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.002,
				GroupRatio:      0.5,
				OriginModelName: ModelDepthVideo,
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess,
	})

	assert.Equal(t, 5000, quota)
}

func TestTaskAdaptorKeepsPrechargeWhenActualDurationIsUnavailable(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Action: ActionDepth,
		Data:   []byte(`{"id":"job_1","status":"completed","progress":100}`),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.002,
				GroupRatio:      1,
				OriginModelName: ModelDepthVideo,
			},
		},
	}

	assert.Zero(t, adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess,
	}))
}

func TestTaskAdaptorCapsReportedDurationAtMaximum(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Action: ActionDepth,
		Data: []byte(
			`{"id":"job_1","status":"completed","progress":100,"fps":1,"frames":1000}`,
		),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.002,
				GroupRatio:      1,
				OriginModelName: ModelDepthVideo,
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess,
	})

	assert.Equal(t, 600000, quota)
}

func TestTaskAdaptorReconcilesSubtitleRemovalToActualDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Action: ActionMedia,
		Data: []byte(
			`{"id":"job_1","status":"completed","progress":100,"fps":24,"frames":73}`,
		),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1,
				OriginModelName: ModelSubtitleRemove,
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{
		Status: model.TaskStatusSuccess,
	})

	assert.Equal(t, 40000, quota)
}
