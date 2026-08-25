package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbortWithOpenAIMessageUsesMiniMaxV2EnvelopeForVideoGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
	c.Set(common.RequestIdKey, "req_minimax_auth_123")

	abortWithOpenAiMessage(c, http.StatusUnauthorized, "invalid API key", types.ErrorCodeAccessDenied)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	response := &dto.MiniMaxAPIErrorResponse{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), response))
	assert.Equal(t, "error", response.Type)
	assert.Equal(t, "authorized_error", response.Error.Type)
	assert.Equal(t, "invalid API key", response.Error.Message)
	assert.NotContains(t, response.Error.Message, "req_minimax_auth_123")
	assert.Equal(t, "401", response.Error.HTTPCode)
	assert.Equal(t, "req_minimax_auth_123", response.RequestID)
	assert.JSONEq(t, `{
		"type": "error",
		"error": {
			"type": "authorized_error",
			"message": "invalid API key",
			"http_code": "401"
		},
		"request_id": "req_minimax_auth_123"
	}`, recorder.Body.String())
}

func TestAbortWithOpenAIMessageKeepsOpenAIEnvelopeForV1Paths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req_openai_auth_456")

	abortWithOpenAiMessage(c, http.StatusUnauthorized, "invalid API key", types.ErrorCodeAccessDenied)

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.JSONEq(t, `{
		"error": {
			"message": "invalid API key (request id: req_openai_auth_456)",
			"type": "new_api_error",
			"code": "access_denied"
		}
	}`, recorder.Body.String())
}

func TestAbortWithOpenAIMessageMasksMiniMaxV2ServerDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
	c.Set(common.RequestIdKey, "req_minimax_server_error")

	abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "select channels failed: password=database-secret")

	assert.True(t, c.IsAborted())
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "Service Unavailable", response.Error.Message)
	assert.NotContains(t, recorder.Body.String(), "database-secret")
	assert.NotContains(t, recorder.Body.String(), "channels")
}

func TestMiniMaxV2PerformanceOverloadUsesOfficialErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousChecker := checkSystemPerformanceForRequest
	checkSystemPerformanceForRequest = func() *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New("database-secret overload detail"), "system_overloaded", http.StatusServiceUnavailable)
	}
	t.Cleanup(func() { checkSystemPerformanceForRequest = previousChecker })

	engine := gin.New()
	engine.Use(SystemPerformanceCheck())
	engine.POST("/v2/video_generation", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "Service Unavailable", response.Error.Message)
	assert.NotContains(t, recorder.Body.String(), "database-secret")
}

func TestMiniMaxV2MemoryRateLimitUsesOfficialErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", 987654321)
		c.Next()
	})
	engine.Use(memoryRateLimitHandler(60, 1, 10))
	engine.GET("/v2/query/video_generation/:task_id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v2/query/video_generation/task_one", nil))
	assert.Equal(t, http.StatusNoContent, first.Code)

	second := httptest.NewRecorder()
	engine.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/v2/query/video_generation/task_two", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &response))
	assert.Equal(t, "429", response.Error.HTTPCode)
	assert.NotEmpty(t, response.Error.Message)
}
