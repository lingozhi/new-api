package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taskdepthmedia "github.com/QuantumNous/new-api/relay/channel/task/depthmedia"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepthMediaRequestConvertInfersProfileAndPreservesWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/media/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
		var request relaycommon.TaskSubmitReq
		require.NoError(t, c.ShouldBindJSON(&request))
		assert.Equal(t, taskdepthmedia.ModelUpscaleFidelity4X, request.Model)
		assert.Equal(t, "https://cdn.example.com/input.png", request.Image)
		assert.Equal(t, "https://client.example.com/hook", request.WebhookURL)
		assert.Equal(t, "secret", request.WebhookSecret)
		assert.Equal(t, "/v1/video/generations", c.Request.URL.Path)
		c.Status(http.StatusNoContent)
	})

	body := `{
		"source_url":"https://cdn.example.com/input.png",
		"operation":"upscale",
		"quality":"fidelity",
		"scale":4,
		"format":"webp",
		"webhook_url":"https://client.example.com/hook",
		"webhook_secret":"secret"
	}`
	request := httptest.NewRequest(http.MethodPost, "/v1/media/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestDepthMediaRequestConvertReplacesReusableBodyStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/media/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
		request, err := getModelFromRequest(c)
		require.NoError(t, err)
		assert.Equal(t, taskdepthmedia.ModelUpscaleFast2X, request.Model)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/media/jobs",
		strings.NewReader(`{
			"source_url":"https://cdn.example.com/input.png",
			"operation":"upscale",
			"quality":"fast",
			"scale":2
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestDepthMediaRequestConvertSupportsUnifiedDepthOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
		var request relaycommon.TaskSubmitReq
		require.NoError(t, c.ShouldBindJSON(&request))
		assert.Equal(t, taskdepthmedia.ModelDepthVideo, request.Model)
		assert.Equal(t, "https://cdn.example.com/input.mp4", request.Image)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/jobs",
		strings.NewReader(`{
			"source_url":"https://cdn.example.com/input.mp4",
			"operation":"depth"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestDepthMediaRequestConvertAcceptsPublicCatalogAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		body      string
		wantModel string
	}{
		{
			name:      "depth video",
			body:      `{"model":"depth-video","source_url":"https://cdn.example.com/input.mp4","operation":"depth"}`,
			wantModel: taskdepthmedia.ModelDepthVideo,
		},
		{
			name:      "background removal",
			body:      `{"model":"background-remove","source_url":"https://cdn.example.com/input.png","operation":"remove_background","quality":"matting"}`,
			wantModel: taskdepthmedia.ModelBackgroundMatting,
		},
		{
			name:      "image upscale",
			body:      `{"model":"image-upscale","source_url":"https://cdn.example.com/input.png","operation":"upscale","quality":"sharp","scale":4}`,
			wantModel: taskdepthmedia.ModelUpscaleSharp4X,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/v1/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
				var request relaycommon.TaskSubmitReq
				require.NoError(t, c.ShouldBindJSON(&request))
				assert.Equal(t, tt.wantModel, request.Model)
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			assert.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}

func TestDepthMediaRequestConvertRejectsUnsupportedProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/media/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})

	body := `{"source_url":"https://cdn.example.com/input.png","operation":"upscale","quality":"fidelity","scale":2}`
	request := httptest.NewRequest(http.MethodPost, "/v1/media/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "unsupported media profile")
}

func TestDepthMediaRequestConvertRejectsMediaModelOnDepthRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/depth/jobs", DepthMediaRequestConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})

	body := `{"model":"background-remove-fast","source_url":"https://cdn.example.com/input.mp4"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/depth/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), taskdepthmedia.ModelDepthVideo)
}
