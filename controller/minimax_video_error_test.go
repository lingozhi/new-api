package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorUsesMiniMaxVideoGenerationV2Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths := []string{
		"/v2/video_generation",
		"/v2/query/video_generation/task_123",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			c.Set(common.RequestIdKey, "req_minimax_123")

			respondTaskError(c, &dto.TaskError{
				Message:    "invalid video request",
				StatusCode: http.StatusUnprocessableEntity,
			})

			assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
			response := &dto.MiniMaxAPIErrorResponse{}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), response))
			assert.Equal(t, "error", response.Type)
			assert.Equal(t, "unprocessable_entity_error", response.Error.Type)
			assert.Equal(t, "invalid video request", response.Error.Message)
			assert.Equal(t, "422", response.Error.HTTPCode)
			assert.Equal(t, "req_minimax_123", response.RequestID)
			assert.JSONEq(t, `{
				"type": "error",
				"error": {
					"type": "unprocessable_entity_error",
					"message": "invalid video request",
					"http_code": "422"
				},
				"request_id": "req_minimax_123"
			}`, recorder.Body.String())
		})
	}
}

func TestRespondTaskErrorMapsInsufficientQuotaToMiniMaxPaymentRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
	c.Set(common.RequestIdKey, "req_minimax_quota")

	respondTaskError(c, &dto.TaskError{
		Code:       string(types.ErrorCodeInsufficientUserQuota),
		Message:    "insufficient quota",
		StatusCode: http.StatusForbidden,
	})

	assert.Equal(t, http.StatusPaymentRequired, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "insufficient_balance_error", response.Error.Type)
	assert.Equal(t, "402", response.Error.HTTPCode)
}

func TestRespondTaskErrorDoesNotExposeInternalDatabaseDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v2/query/video_generation/task_123", nil)
	c.Set(common.RequestIdKey, "req_minimax_internal")

	respondTaskError(c, &dto.TaskError{
		Code:       "get_task_failed",
		Message:    `pq: relation "tasks" does not exist; password=database-secret`,
		StatusCode: http.StatusInternalServerError,
	})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "Internal Server Error", response.Error.Message)
	assert.NotContains(t, recorder.Body.String(), "tasks")
	assert.NotContains(t, recorder.Body.String(), "database-secret")
}

func TestRespondTaskErrorDoesNotExposeWorkflowProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)

	respondTaskError(c, &dto.TaskError{
		Message:    "AutoDL rejected the workflow submission",
		StatusCode: http.StatusUnprocessableEntity,
	})

	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	assert.NotContains(t, strings.ToLower(recorder.Body.String()), "autodl")
	assert.Contains(t, recorder.Body.String(), "Video generation request failed")
}
