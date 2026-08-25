package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidGzipVideoGenerationBodyUsesMiniMaxEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(DecompressRequestMiddleware())
	engine.POST("/v2/video_generation", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v2/video_generation", bytes.NewReader([]byte("not a gzip stream")))
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "bad_request_error", response.Error.Type)
	assert.Equal(t, "invalid compressed request body", response.Error.Message)
	assert.Equal(t, "400", response.Error.HTTPCode)
}

func TestInvalidZstdVideoGenerationBodyUsesMiniMaxEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(DecompressRequestMiddleware())
	engine.POST("/v2/video_generation", func(c *gin.Context) {
		_, _, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v2/video_generation", bytes.NewReader([]byte("not a zstd stream")))
	request.Header.Set("Content-Encoding", "zstd")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "bad_request_error", response.Error.Type)
	assert.Equal(t, "400", response.Error.HTTPCode)
}

func TestInvalidGzipV1BodyKeepsEmptyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(DecompressRequestMiddleware())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not a gzip stream")))
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Empty(t, recorder.Body.String())
}
