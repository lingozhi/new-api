package controller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoDLAudioWAVFixture() []byte {
	return []byte{'R', 'I', 'F', 'F', 4, 0, 0, 0, 'W', 'A', 'V', 'E', 0, 0, 0, 0}
}

func autoDLAudioTestTokenID(userID int) int {
	return userID + 1000
}

func autoDLAudioSuccessTask(userID int) *model.Task {
	return &model.Task{
		TaskID:     "task_audio_success",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     userID,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusSuccess,
		SubmitTime: time.Now().Unix(),
		FinishTime: time.Now().Unix(),
		Properties: model.Properties{OriginModelName: constant.AutoDLModelIndexTTS2},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-audio-task",
			ResultURL:      "https://media.example.com/generated.wav?signature=short-lived",
			TokenId:        autoDLAudioTestTokenID(userID),
		},
	}
}

func newAutoDLAudioControllerContext(userID int) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/audio/speech/task_audio_success", nil)
	context.Set("id", userID)
	common.SetContextKey(context, constant.ContextKeyTokenId, autoDLAudioTestTokenID(userID))
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, false)
	return context, recorder
}

func installAutoDLAudioFetch(
	t *testing.T,
	body []byte,
	fetchErr error,
	validatedURL *string,
) *int {
	t.Helper()
	previousFetch := fetchAutoDLAudioResult
	requestCount := 0
	fetchAutoDLAudioResult = func(_ context.Context, resultURL string, maxBytes int64) (common.BodyStorage, error) {
		requestCount++
		assert.Equal(t, "https://media.example.com/generated.wav?signature=short-lived", resultURL)
		assert.Equal(t, int64(maxAutoDLAudioResultBytes), maxBytes)
		if validatedURL != nil {
			*validatedURL = resultURL
		}
		if fetchErr != nil {
			return nil, fetchErr
		}
		return common.CreateDiskBodyStorageFromReader(bytes.NewReader(body), maxBytes)
	}
	t.Cleanup(func() {
		fetchAutoDLAudioResult = previousFetch
	})
	return &requestCount
}

func TestAutoDLAudioSpeechFacadeReturnsValidatedWAV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wav := autoDLAudioWAVFixture()
	validatedURL := ""
	requestCount := installAutoDLAudioFetch(t, wav, nil, &validatedURL)
	context, recorder := newAutoDLAudioControllerContext(71)
	task := autoDLAudioSuccessTask(71)

	writeAutoDLAudioSpeechTask(context, task)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, wav, recorder.Body.Bytes())
	assert.Equal(t, task.PrivateData.ResultURL, validatedURL)
	assert.Equal(t, 1, *requestCount)
	assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "16", recorder.Header().Get("Content-Length"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, task.TaskID, recorder.Header().Get("X-New-Api-Task-ID"))
	assert.Equal(t, "/v1/audio/speech/"+task.TaskID, recorder.Header().Get("Location"))
}

func TestAutoDLAudioSpeechFacadeRejectsInvalidDownloadResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestCount := installAutoDLAudioFetch(t, nil, errors.New("generated audio is not a valid WAV"), nil)
	context, recorder := newAutoDLAudioControllerContext(72)

	writeAutoDLAudioResult(context, autoDLAudioSuccessTask(72))

	assert.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	assert.Equal(t, 1, *requestCount)
	var response struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "invalid_audio_result", response.Error.Code)
	assert.Empty(t, recorder.Header().Get("Content-Length"))
}

func TestAutoDLAudioSpeechFacadeReturnsDurablePendingHandle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, recorder := newAutoDLAudioControllerContext(74)
	task := &model.Task{
		TaskID:     "task_audio_pending",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     74,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusCheckpointPending,
		SubmitTime: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: constant.AutoDLModelIndexTTS2,
		},
		PrivateData: model.TaskPrivateData{TokenId: autoDLAudioTestTokenID(74)},
	}

	writeAutoDLAudioSpeechTask(context, task)

	assert.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	assert.Equal(t, "2", recorder.Header().Get("Retry-After"))
	assert.Equal(t, task.TaskID, recorder.Header().Get("X-New-Api-Task-ID"))
	assert.Equal(t, "/v1/audio/speech/"+task.TaskID, recorder.Header().Get("Location"))
	assert.JSONEq(t, `{"task_id":"task_audio_pending","status":"checkpoint_pending"}`, recorder.Body.String())
}

func TestAutoDLAudioSpeechFacadeHidesTasksOutsideOwnershipBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		mutate func(*gin.Context, *model.Task)
	}{
		{name: "different user", mutate: func(_ *gin.Context, task *model.Task) { task.UserId++ }},
		{name: "different platform", mutate: func(_ *gin.Context, task *model.Task) { task.Platform = constant.TaskPlatformSuno }},
		{name: "different action", mutate: func(_ *gin.Context, task *model.Task) { task.Action = constant.TaskActionVideoGenerationV2 }},
		{name: "different token", mutate: func(_ *gin.Context, task *model.Task) { task.PrivateData.TokenId++ }},
		{name: "outside seven day query window", mutate: func(_ *gin.Context, task *model.Task) {
			task.SubmitTime = time.Now().Add(-8 * 24 * time.Hour).Unix()
		}},
		{name: "model no longer allowed", mutate: func(context *gin.Context, _ *model.Task) {
			common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"another-model": true})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, recorder := newAutoDLAudioControllerContext(75)
			task := autoDLAudioSuccessTask(75)
			test.mutate(context, task)

			writeAutoDLAudioSpeechTask(context, task)

			assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
			assert.Empty(t, recorder.Header().Get("X-New-Api-Task-ID"))
			assert.NotContains(t, recorder.Body.String(), task.TaskID)
			assert.Contains(t, recorder.Body.String(), "task_not_found")
		})
	}
}

func TestAutoDLAudioSpeechWaitTimeoutReturnsRecoverablePendingHandle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousPoll := pollAutoDLAudioTask
	previousWaitTimeout := autoDLAudioWaitTimeout
	pollAutoDLAudioTask = func(context.Context, *model.Task) (*model.Task, error) {
		return nil, errors.New("temporary poll failure")
	}
	autoDLAudioWaitTimeout = 5 * time.Millisecond
	t.Cleanup(func() {
		pollAutoDLAudioTask = previousPoll
		autoDLAudioWaitTimeout = previousWaitTimeout
	})

	context, recorder := newAutoDLAudioControllerContext(76)
	task := autoDLAudioSuccessTask(76)
	task.TaskID = "task_audio_waiting"
	task.Status = model.TaskStatusQueued
	task.FinishTime = 0
	task.PrivateData.ResultURL = ""

	writeAutoDLAudioSpeechTask(context, task)

	assert.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	assert.Equal(t, "2", recorder.Header().Get("Retry-After"))
	assert.Equal(t, "/v1/audio/speech/"+task.TaskID, recorder.Header().Get("Location"))
	assert.JSONEq(t, `{"task_id":"task_audio_waiting","status":"queued"}`, recorder.Body.String())
}
