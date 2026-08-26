package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	autoDLAudioClientRequestIDDomain   = "autodl-audio-idempotency:v1:"
	autoDLAudioClientRequestHashDomain = "autodl-audio-request:v1:"
	autoDLAudioPollInterval            = time.Second
	autoDLAudioDownloadTimeout         = 90 * time.Second
	maxAutoDLAudioResultBytes          = 64 << 20
)

var (
	fetchAutoDLAudioResult = service.FetchValidatedWAV
	pollAutoDLAudioTask    = service.PollTaskOnce
	autoDLAudioWaitTimeout = 5 * time.Minute
)

// ReplayAutoDLAudioSpeech resolves an idempotent IndexTTS2 request before
// channel selection or billing. The raw key, text, and reference audio never
// enter the task row; only domain-separated hashes are persisted.
func ReplayAutoDLAudioSpeech(c *gin.Context) {
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		c.Next()
		return
	}
	mediaType, _, mediaTypeErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaTypeErr != nil || mediaType != gin.MIMEJSON {
		// Only official JSON requests participate in replay. Non-JSON bodies
		// continue to channel selection, where AutoDL rejects them before
		// billing/provider dispatch.
		c.Next()
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		c.Next()
		return
	}
	if storage.Size() > common.MaxAutoDLWorkflowBodyBytes {
		// The selected channel owns the model-specific error. Do not materialize
		// an oversized request merely because the caller supplied a replay key.
		c.Next()
		return
	}

	request := &dto.IndexTTS2SpeechRequest{}
	if err := common.UnmarshalBodyReusable(c, request); err != nil || request.Model != constant.AutoDLModelIndexTTS2 {
		// The normal relay path owns malformed JSON and non-AutoDL models.
		c.Next()
		return
	}
	common.SetContextKey(c, constant.ContextKeyValidatedAutoDLAudioRequest, request)
	if len(idempotencyKey) > 256 {
		writeAutoDLAudioError(c, http.StatusBadRequest, "invalid_request_error", "idempotency_key_too_long", "Idempotency-Key is too long")
		c.Abort()
		return
	}
	if len(request.Metadata) > 0 {
		var metadata any
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			c.Next()
			return
		}
		canonicalMetadata, err := common.Marshal(metadata)
		if err != nil {
			writeAutoDLAudioError(c, http.StatusInternalServerError, "server_error", "idempotency_hash_failed", http.StatusText(http.StatusInternalServerError))
			c.Abort()
			return
		}
		request.Metadata = canonicalMetadata
	}
	canonicalRequest, err := common.Marshal(request)
	if err != nil {
		writeAutoDLAudioError(c, http.StatusInternalServerError, "server_error", "idempotency_hash_failed", http.StatusText(http.StatusInternalServerError))
		c.Abort()
		return
	}

	clientRequestID := common.Sha256([]byte(autoDLAudioClientRequestIDDomain + idempotencyKey))
	requestHash := common.Sha256(append([]byte(autoDLAudioClientRequestHashDomain), canonicalRequest...))
	c.Set(autoDLClientRequestIDContextKey, clientRequestID)
	c.Set(autoDLClientRequestHashContextKey, requestHash)

	existing, exists, err := model.GetTaskByClientRequestID(constant.TaskPlatformAutoDL, c.GetInt("id"), clientRequestID)
	if err != nil {
		writeAutoDLAudioError(c, http.StatusInternalServerError, "server_error", "idempotency_lookup_failed", http.StatusText(http.StatusInternalServerError))
		c.Abort()
		return
	}
	if !exists {
		request = nil
		canonicalRequest = nil
		c.Next()
		return
	}
	if !autoDLAudioTaskAuthorized(c, existing) {
		writeAutoDLAudioError(c, http.StatusNotFound, "invalid_request_error", "task_not_found", "Task not found")
		c.Abort()
		return
	}
	if existing.PrivateData.ClientRequestHash != requestHash {
		writeAutoDLAudioError(c, http.StatusConflict, "invalid_request_error", "idempotency_conflict", "Idempotency-Key was already used with a different request")
		c.Abort()
		return
	}

	c.Header("Idempotency-Replayed", "true")
	*request = dto.IndexTTS2SpeechRequest{}
	common.SetContextKey(c, constant.ContextKeyValidatedAutoDLAudioRequest, nil)
	writeAutoDLAudioSpeechTask(c, existing)
	c.Abort()
}

// RelayAudioSpeech keeps ordinary OpenAI-compatible TTS channels on the
// synchronous audio relay and sends only the selected AutoDL workflow through
// the durable asynchronous bridge.
func RelayAudioSpeech(c *gin.Context) {
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeAutoDL {
		Relay(c, types.RelayFormatOpenAIAudio)
		return
	}
	if strings.TrimSpace(c.GetHeader("Idempotency-Key")) == "" {
		writeAutoDLAudioError(c, http.StatusBadRequest, "invalid_request_error", "idempotency_key_required", "Idempotency-Key is required for indextts2-v1")
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != gin.MIMEJSON {
		writeAutoDLAudioError(c, http.StatusUnsupportedMediaType, "invalid_request_error", "unsupported_media_type", "indextts2-v1 requires Content-Type: application/json")
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		writeAutoDLAudioError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request", "Failed to read request body")
		return
	}
	if storage.Size() > common.MaxAutoDLWorkflowBodyBytes {
		writeAutoDLAudioError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large", "request body must not exceed 64 MiB")
		return
	}

	if setting.ShouldCheckPromptSensitive() {
		request := &dto.IndexTTS2SpeechRequest{}
		if value, exists := common.GetContextKey(c, constant.ContextKeyValidatedAutoDLAudioRequest); exists && value != nil {
			if cached, ok := value.(*dto.IndexTTS2SpeechRequest); ok && cached != nil {
				request = cached
			}
		}
		if request.Model != "" || common.UnmarshalBodyReusable(c, request) == nil {
			contains, words := service.CheckSensitiveText(request.EffectivePromptText())
			if contains {
				logger.LogWarn(c, fmt.Sprintf("user sensitive words detected in AutoDL audio input: matches=%d", len(words)))
				writeAutoDLAudioError(c, http.StatusBadRequest, "invalid_request_error", string(types.ErrorCodeSensitiveWordsDetected), "The input contains sensitive words")
				return
			}
		}
	}

	RelayTask(c)
}

// GetAutoDLAudioSpeechTask is a recovery endpoint for a POST that outlives the
// client connection or synchronous wait window.
func GetAutoDLAudioSpeechTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		writeAutoDLAudioError(c, http.StatusBadRequest, "invalid_request_error", "invalid_task_id", "task_id is required")
		return
	}
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		writeAutoDLAudioError(c, http.StatusInternalServerError, "server_error", "task_lookup_failed", http.StatusText(http.StatusInternalServerError))
		return
	}
	if !exists || task == nil || task.Platform != constant.TaskPlatformAutoDL || task.Action != constant.TaskActionAudioSpeech || !relay.IsAutoDLTaskWithinQueryWindow(task, time.Now()) {
		writeAutoDLAudioError(c, http.StatusNotFound, "invalid_request_error", "task_not_found", "Task not found")
		return
	}
	writeAutoDLAudioSpeechTask(c, task)
}

func writeAutoDLAudioSpeechTask(c *gin.Context, task *model.Task) {
	if task == nil || task.UserId != c.GetInt("id") || task.Platform != constant.TaskPlatformAutoDL || task.Action != constant.TaskActionAudioSpeech ||
		!autoDLAudioTaskAuthorized(c, task) || !relay.IsAutoDLTaskWithinQueryWindow(task, time.Now()) {
		writeAutoDLAudioError(c, http.StatusNotFound, "invalid_request_error", "task_not_found", "Task not found")
		return
	}
	// The POST body can contain two large base64 audio references. Submission no
	// longer needs it, so release the disk spool before the synchronous wait.
	common.CleanupBodyStorage(c)

	c.Header("X-New-Api-Task-ID", task.TaskID)
	c.Header("Location", "/v1/audio/speech/"+task.TaskID)
	current := task
	if current.Status == model.TaskStatusReserving || current.Status == model.TaskStatusCheckpointPending || current.GetUpstreamTaskID() == "" {
		writeAutoDLAudioPending(c, current)
		return
	}

	waitCtx, cancel := context.WithTimeout(c.Request.Context(), autoDLAudioWaitTimeout)
	defer cancel()
	var lastPollErr error
	for {
		switch current.Status {
		case model.TaskStatusSuccess:
			writeAutoDLAudioResult(c, current)
			return
		case model.TaskStatusFailure:
			writeAutoDLAudioError(c, http.StatusBadGateway, "server_error", "audio_generation_failed", "Audio generation failed")
			return
		}

		latest, err := pollAutoDLAudioTask(waitCtx, current)
		if err == nil {
			current = latest
			lastPollErr = nil
		} else {
			lastPollErr = err
			logger.LogWarn(c, fmt.Sprintf("AutoDL audio task poll failed: task_id=%s error=%s", current.TaskID, common.MaskSensitiveInfo(err.Error())))
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.Canceled) {
				return
			}
			if lastPollErr != nil {
				logger.LogWarn(c, fmt.Sprintf("AutoDL audio synchronous wait expired after poll errors: task_id=%s", current.TaskID))
			}
			writeAutoDLAudioPending(c, current)
			return
		case <-time.After(autoDLAudioPollInterval):
		}
	}
}

func autoDLAudioTaskAuthorized(c *gin.Context, task *model.Task) bool {
	if task == nil {
		return false
	}
	currentTokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if currentTokenID <= 0 || task.PrivateData.TokenId <= 0 || currentTokenID != task.PrivateData.TokenId {
		return false
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return true
	}
	value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	if !exists {
		return false
	}
	modelLimits, ok := value.(map[string]bool)
	if !ok {
		return false
	}
	modelName := ratio_setting.FormatMatchingModelName(task.Properties.OriginModelName)
	_, allowed := modelLimits[modelName]
	return allowed
}

func writeAutoDLAudioPending(c *gin.Context, task *model.Task) {
	c.Header("Retry-After", "2")
	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.TaskID,
		"status":  strings.ToLower(string(task.Status)),
	})
}

func writeAutoDLAudioResult(c *gin.Context, task *model.Task) {
	if task.FinishTime > 0 && time.Now().Unix()-task.FinishTime >= int64(30*time.Second/time.Second) {
		refreshCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		relay.RefreshAutoDLSuccessTask(refreshCtx, task)
		cancel()
	}

	fetchCtx, cancel := context.WithTimeout(c.Request.Context(), autoDLAudioDownloadTimeout)
	defer cancel()
	resultURL := strings.TrimSpace(task.GetResultURL())
	audio, err := fetchAutoDLAudioResult(fetchCtx, resultURL, maxAutoDLAudioResultBytes)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf(
			"AutoDL audio result fetch failed: task_id=%s channel_id=%d error=%s",
			task.TaskID,
			task.ChannelId,
			common.MaskSensitiveInfo(err.Error()),
		))
		writeAutoDLAudioError(c, http.StatusBadGateway, "server_error", "invalid_audio_result", "Failed to fetch valid generated WAV audio")
		return
	}
	defer audio.Close()
	if _, err := audio.Seek(0, io.SeekStart); err != nil {
		writeAutoDLAudioError(c, http.StatusBadGateway, "server_error", "audio_download_failed", "Failed to read generated audio")
		return
	}

	c.Header("Content-Type", "audio/wav")
	c.Header("Content-Length", fmt.Sprintf("%d", audio.Size()))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, audio); err != nil {
		logger.LogWarn(c, fmt.Sprintf("AutoDL audio result stream failed: task_id=%s", task.TaskID))
	}
}

func writeAutoDLAudioError(c *gin.Context, status int, errorType, code, message string) {
	c.JSON(status, gin.H{"error": types.OpenAIError{
		Message: message,
		Type:    errorType,
		Code:    code,
	}})
}
