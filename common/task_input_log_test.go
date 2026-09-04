package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskInputLogPreservesInputAndDoesNotChangeForwardedBody(t *testing.T) {
	body := `{"model":"seedance-2.5","prompt":"A calm forest","duration":5,"generate_audio":false,"seed":0,"reference_images":[{"url":"https://cdn.example/img.png?signature=private"}],"metadata":{"api_key":"secret"},"image":"data:image/png;base64,abc"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	log := TaskInputLog(c)
	var got map[string]any
	require.NoError(t, UnmarshalJsonStr(log, &got))
	assert.Equal(t, "A calm forest", got["prompt"])
	assert.Equal(t, false, got["generate_audio"])
	assert.Equal(t, float64(0), got["seed"])
	assert.NotContains(t, log, "signature=private")
	assert.NotContains(t, log, "\"secret\"")
	assert.NotContains(t, log, "base64,abc")
	var original map[string]any
	require.NoError(t, UnmarshalBodyReusable(c, &original))
	restored, err := Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(restored))
}

func TestTaskInputLogMultipartOmitsFileBytes(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "Animate this photo"))
	file, err := writer.CreateFormFile("image", "photo.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("private-file-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	log := TaskInputLog(c)
	assert.Contains(t, log, "Animate this photo")
	assert.Contains(t, log, "photo.png")
	assert.NotContains(t, log, "private-file-bytes")
}
