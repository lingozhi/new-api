package depthmedia

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	info := &relaycommon.RelayInfo{ChannelBaseUrl: "https://modal.example.com", Action: ActionMedia}
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
	info.ApiKey = "upstream-secret"
	require.NoError(t, adaptor.BuildRequestHeader(c, request, info))
	assert.Equal(t, "Bearer upstream-secret", request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
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
}
