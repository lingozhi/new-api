package middleware

import (
	"context"
	"fmt"
	"io"
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

func TestUploadIdleTimeoutReturnsCodexRetryableResponse(t *testing.T) {
	require.NoError(t, i18n.Init())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	abortWithClientDisconnect(
		c,
		fmt.Errorf("read request body: %w (1m0s)", common.ErrUploadIdleTimeout),
		time.Now().Add(-time.Minute),
	)

	assert.Equal(t, http.StatusServiceUnavailable, c.Writer.Status())
	assert.Equal(t, "close", recorder.Header().Get("Connection"))
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

func TestUploadIdleTimeoutResponseReachesHTTPClient(t *testing.T) {
	require.NoError(t, i18n.Init())
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Request.Body = &idleTimeoutBody{
			ReadCloser: c.Request.Body,
			rc:         http.NewResponseController(c.Writer),
			timeout:    50 * time.Millisecond,
		}
		startedAt := time.Now()
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			abortWithClientDisconnect(c, err, startedAt)
			return
		}
		c.Status(http.StatusNoContent)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	bodyReader, bodyWriter := io.Pipe()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", bodyReader)
	require.NoError(t, err)
	request.ContentLength = 1 << 20
	releaseUpload := make(chan struct{})
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_, _ = bodyWriter.Write([]byte(`{"model":"gpt-5.6-sol"`))
		<-releaseUpload
		_ = bodyWriter.Close()
	}()

	response, err := server.Client().Do(request)
	close(releaseUpload)
	<-writeDone
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	assert.Equal(t, "1", response.Header.Get("Retry-After"))
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(responseBody), "read_request_body_failed")
}
