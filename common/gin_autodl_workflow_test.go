package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoDLWorkflowJSONAlwaysUsesDiskBodyStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{constant.AutoDLVideoGenerationV2Path, constant.AutoDLAudioSpeechPath} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"test"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { CleanupBodyStorage(c) })

			storage, err := GetBodyStorage(c)
			require.NoError(t, err)
			assert.True(t, storage.IsDisk())
		})
	}
}

func TestRequestBodyLimitKeepsSharedAudioSpeechConfiguration(t *testing.T) {
	configured := MaxAutoDLWorkflowBodyBytes + 17
	assert.Equal(t, configured, RequestBodyLimitBytes(constant.AutoDLAudioSpeechPath, configured))
	assert.Equal(t, MaxAutoDLWorkflowBodyBytes, RequestBodyLimitBytes(constant.AutoDLVideoGenerationV2Path, configured))
}
