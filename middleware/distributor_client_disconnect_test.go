package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadIdleTimeoutReturnsStandardRetryableResponse(t *testing.T) {
	require.NoError(t, i18n.Init())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	abortWithClientDisconnect(
		c,
		fmt.Errorf("read request body: %w (1m0s)", common.ErrUploadIdleTimeout),
		time.Now().Add(-time.Minute),
	)

	assert.Equal(t, http.StatusRequestTimeout, c.Writer.Status())
	assert.Equal(t, "1", recorder.Header().Get("Retry-After"))
	var response struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Contains(t, response.Error.Message, "upload timed out")
	assert.Equal(t, "new_api_error", response.Error.Type)
	assert.Equal(t, "read_request_body_failed", response.Error.Code)
}

func TestActualClientDisconnectRemains499(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	abortWithClientDisconnect(c, context.Canceled, time.Now())

	assert.Equal(t, StatusClientClosedRequest, c.Writer.Status())
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("Retry-After"))
}
