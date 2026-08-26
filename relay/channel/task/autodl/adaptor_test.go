package autodl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reportedSizeBodyStorage struct {
	*bytes.Reader
	data []byte
	size int64
}

func newReportedSizeBodyStorage(data []byte, size int64) *reportedSizeBodyStorage {
	return &reportedSizeBodyStorage{Reader: bytes.NewReader(data), data: data, size: size}
}

func (s *reportedSizeBodyStorage) Close() error           { return nil }
func (s *reportedSizeBodyStorage) Bytes() ([]byte, error) { return s.data, nil }
func (s *reportedSizeBodyStorage) Size() int64            { return s.size }
func (s *reportedSizeBodyStorage) IsDisk() bool           { return false }

func pointer[T any](value T) *T {
	return &value
}

func newMiniMaxRequest(duration int, ratio string, content ...dto.MiniMaxVideoContentItem) *dto.MiniMaxVideoGenerationV2Request {
	return &dto.MiniMaxVideoGenerationV2Request{
		Model:      "MiniMax-H3",
		Content:    dto.MiniMaxVideoContentItems(content),
		Resolution: pointer("768P"),
		Duration:   pointer(duration),
		Ratio:      pointer(ratio),
	}
}

func textContent(prompt string) dto.MiniMaxVideoContentItem {
	return dto.MiniMaxVideoContentItem{Type: "text", Text: prompt}
}

func imageContent(role, mediaURL string) dto.MiniMaxVideoContentItem {
	return dto.MiniMaxVideoContentItem{
		Type:     "image_url",
		Role:     role,
		ImageURL: &dto.MiniMaxVideoMedia{URL: mediaURL},
	}
}

func audioContent(mediaURL string) dto.MiniMaxVideoContentItem {
	return dto.MiniMaxVideoContentItem{
		Type:     "audio_url",
		Role:     miniMaxRoleReferenceAudio,
		AudioURL: &dto.MiniMaxVideoMedia{URL: mediaURL},
	}
}

func TestTaskAdaptorMapsMiniMaxTextToVideoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"A paper boat"},{"type":"text","text":"floating at sunset"}],
		"resolution":"768P",
		"duration":6,
		"ratio":"16:9"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://autodl.example/",
			ApiKey:         "autodl-token",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, constant.TaskActionVideoGenerationV2, info.Action)
	assert.Equal(t, workflowTextToVideo, adaptor.workflowID)
	assert.Equal(t, &relaycommon.TaskVideoProperties{
		Resolution: "768P",
		Duration:   6,
		Ratio:      "16:9",
	}, info.Video)
	assert.Equal(t, map[string]float64{"seconds": 6}, adaptor.EstimateBilling(c, info))

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "A paper boat\nfloating at sunset", payload["prompt"])
	assert.Equal(t, float64(6), payload["duration"])
	assert.Equal(t, "768p横", payload["resolution"])
	assert.NotContains(t, payload, "first_frame")
	assert.NotContains(t, payload, "last_frame")
}

func TestTaskAdaptorEnforcesOfficialRequestBodySizeLimit(t *testing.T) {
	requestBody := []byte(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"A paper boat"}],
		"resolution":"768P",
		"duration":6,
		"ratio":"16:9"
	}`)

	newContext := func(reportedSize int64) (*gin.Context, *relaycommon.RelayInfo) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.KeyBodyStorage, newReportedSizeBodyStorage(requestBody, reportedSize))
		return c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	}

	overLimitContext, overLimitInfo := newContext(maxRequestBodyBytes + 1)
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(overLimitContext, overLimitInfo)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusRequestEntityTooLarge, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "64 MiB")

	atLimitContext, atLimitInfo := newContext(maxRequestBodyBytes)
	assert.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(atLimitContext, atLimitInfo))
}

func TestTaskAdaptorValidatesAndStoresMiniMaxCallbackWithoutForwardingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_SECRET", "callback-test-secret")
	t.Setenv("ASYNC_IMAGE_ENCRYPTED_WRITES_ENABLED", "true")
	previousUpdateTask := constant.UpdateTask
	constant.UpdateTask = true
	t.Cleanup(func() { constant.UpdateTask = previousUpdateTask })

	previousVerifier := verifyJSONWebhookChallenge
	verificationCalls := 0
	verifyJSONWebhookChallenge = func(_ context.Context, callbackURL string, userID int, tokenID int) error {
		verificationCalls++
		assert.Equal(t, "https://callbacks.example.com/minimax?token=secret", callbackURL)
		assert.Equal(t, 17, userID)
		assert.Equal(t, 29, tokenID)
		return nil
	}
	t.Cleanup(func() { verifyJSONWebhookChallenge = previousVerifier })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 17)
	common.SetContextKey(c, constant.ContextKeyTokenId, 29)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"A paper boat"}],
		"resolution":"768P",
		"duration":6,
		"ratio":"16:9",
		"callback_url":"  https://callbacks.example.com/minimax?token=secret  "
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, 1, verificationCalls, "channel retries must reuse the successful challenge")
	require.NotNil(t, adaptor.request.CallbackURL)
	assert.Equal(t, "https://callbacks.example.com/minimax?token=secret", *adaptor.request.CallbackURL)
	taskRequest, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, "https://callbacks.example.com/minimax?token=secret", taskRequest.WebhookURL)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "callback")
	assert.NotContains(t, string(data), "token=secret")
}

func TestTaskAdaptorRejectsMiniMaxCallbackWhenSecureWorkerIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("ASYNC_IMAGE_ENCRYPTED_WRITES_ENABLED", "true")
	previousUpdateTask := constant.UpdateTask
	constant.UpdateTask = true
	t.Cleanup(func() { constant.UpdateTask = previousUpdateTask })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"A paper boat"}],
		"resolution":"768P",
		"duration":6,
		"ratio":"16:9",
		"callback_url":"https://callbacks.example.com/minimax"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	assert.Equal(t, "callback_unavailable", taskErr.Code)
}

func TestTaskAdaptorRejectsInvalidMiniMaxCallbackBeforeSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_SECRET", "callback-test-secret")
	t.Setenv("ASYNC_IMAGE_ENCRYPTED_WRITES_ENABLED", "true")
	previousUpdateTask := constant.UpdateTask
	constant.UpdateTask = true
	t.Cleanup(func() { constant.UpdateTask = previousUpdateTask })

	previousVerifier := verifyJSONWebhookChallenge
	t.Cleanup(func() { verifyJSONWebhookChallenge = previousVerifier })

	t.Run("URL exceeds the documented limit", func(t *testing.T) {
		verificationCalls := 0
		verifyJSONWebhookChallenge = func(context.Context, string, int, int) error {
			verificationCalls++
			return nil
		}
		request := newMiniMaxRequest(6, "16:9", textContent("A paper boat"))
		request.CallbackURL = pointer("https://callbacks.example.com/" + strings.Repeat("a", maxCallbackURLCharacters))
		body, err := common.Marshal(request)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		assert.Contains(t, taskErr.Message, "2048 characters")
		assert.Zero(t, verificationCalls)
	})

	t.Run("challenge is not echoed", func(t *testing.T) {
		verifyJSONWebhookChallenge = func(context.Context, string, int, int) error {
			return errors.New("challenge mismatch")
		}
		request := newMiniMaxRequest(6, "16:9", textContent("A paper boat"))
		request.CallbackURL = pointer("https://callbacks.example.com/minimax")
		body, err := common.Marshal(request)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		assert.Contains(t, taskErr.Message, "challenge mismatch")
	})

	t.Run("challenge admission is rate limited", func(t *testing.T) {
		verifyJSONWebhookChallenge = func(context.Context, string, int, int) error {
			return &service.WebhookChallengeAdmissionError{RetryAfterSeconds: 7}
		}
		request := newMiniMaxRequest(6, "16:9", textContent("A paper boat"))
		request.CallbackURL = pointer("https://callbacks.example.com/minimax")
		body, err := common.Marshal(request)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
		assert.Equal(t, "callback_rate_limit_exceeded", taskErr.Code)
		assert.Equal(t, "7", recorder.Header().Get("Retry-After"))
	})
}

func TestBuildWorkflowRequestMapsFirstAndLastFrames(t *testing.T) {
	tests := []struct {
		name       string
		firstFrame dto.MiniMaxVideoContentItem
	}{
		{name: "explicit first-frame role", firstFrame: imageContent(miniMaxRoleFirstFrame, "https://cdn.example.com/first.png")},
		{name: "omitted role defaults to first frame", firstFrame: imageContent("", "https://cdn.example.com/first.png")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newMiniMaxRequest(
				8,
				"16:9",
				textContent("The camera moves through the scene"),
				test.firstFrame,
				imageContent(miniMaxRoleLastFrame, "https://cdn.example.com/last.png"),
			)

			workflowID, payload, properties, err := buildWorkflowRequest(request)
			require.NoError(t, err)
			assert.Equal(t, workflowFirstLastFrame, workflowID)
			assert.Equal(t, "The camera moves through the scene", payload["prompt"])
			assert.Equal(t, 8, payload["duration"])
			assert.Equal(t, "https://cdn.example.com/first.png", payload["first_frame"])
			assert.Equal(t, "https://cdn.example.com/last.png", payload["last_frame"])
			assert.Equal(t, "768p横", payload["resolution"])
			assert.Equal(t, 2, properties.InputImageCount)
		})
	}
}

func TestBuildWorkflowRequestSelectsReferenceMediaWorkflowByDuration(t *testing.T) {
	tests := []struct {
		name        string
		duration    int
		ratio       string
		content     []dto.MiniMaxVideoContentItem
		workflowID  string
		resolution  string
		inputImages int
	}{
		{
			name:        "reference image up to ten seconds",
			duration:    10,
			ratio:       "1:1",
			content:     []dto.MiniMaxVideoContentItem{textContent("Animate the subject"), imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref.png")},
			workflowID:  workflowReferenceImages,
			resolution:  "768p(1:1)",
			inputImages: 1,
		},
		{
			name:        "reference image over ten seconds",
			duration:    15,
			ratio:       "1:1",
			content:     []dto.MiniMaxVideoContentItem{textContent("Animate the subject"), imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref.png")},
			workflowID:  workflowReferenceImages15s,
			resolution:  "768p(1:1)",
			inputImages: 1,
		},
		{
			name:     "multiple reference images and audio up to ten seconds",
			duration: 10,
			ratio:    "16:9",
			content: []dto.MiniMaxVideoContentItem{
				textContent("Make the character speak"),
				imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref-0.png"),
				imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref-1.png"),
				audioContent("https://cdn.example.com/voice.wav"),
			},
			workflowID:  workflowReferenceImageAudio,
			resolution:  "768p横",
			inputImages: 2,
		},
		{
			name:     "multiple reference images and audio over ten seconds",
			duration: 15,
			ratio:    "9:16",
			content: []dto.MiniMaxVideoContentItem{
				textContent("Make the character speak"),
				imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref-0.png"),
				imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref-1.png"),
				audioContent("https://cdn.example.com/voice.wav"),
			},
			workflowID:  workflowReferenceImageAudio15s,
			resolution:  "768p竖",
			inputImages: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflowID, payload, properties, err := buildWorkflowRequest(newMiniMaxRequest(test.duration, test.ratio, test.content...))
			require.NoError(t, err)
			assert.Equal(t, test.workflowID, workflowID)
			assert.Equal(t, test.resolution, payload["resolution"])
			assert.Equal(t, test.duration, payload["duration"])
			assert.Equal(t, test.ratio, properties.Ratio)
			assert.Equal(t, test.inputImages, properties.InputImageCount)
			if strings.Contains(test.name, "multiple") {
				assert.Equal(t, "https://cdn.example.com/ref-0.png", payload["ref_image_0"])
				assert.Equal(t, "https://cdn.example.com/ref-1.png", payload["ref_image_1"])
			} else {
				assert.Equal(t, "https://cdn.example.com/ref.png", payload["ref_image_0"])
			}
			if strings.Contains(test.name, "audio") {
				assert.Equal(t, "https://cdn.example.com/voice.wav", payload["ref_audio_0"])
			} else {
				assert.NotContains(t, payload, "ref_audio_0")
			}
		})
	}
}

func TestBuildWorkflowRequestDistinguishesReferencePairFromAudioSync(t *testing.T) {
	referencePrompt := strings.Repeat("a", 8000)
	referenceRequest := newMiniMaxRequest(
		15,
		"9:16",
		textContent(referencePrompt),
		imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/portrait.png"),
		audioContent("https://cdn.example.com/voice.wav"),
	)
	referenceRequest.Seed = pointer(int64(0))
	workflowID, payload, properties, err := buildWorkflowRequest(referenceRequest)
	require.NoError(t, err)
	assert.Equal(t, workflowReferenceImageAudio15s, workflowID)
	assert.Equal(t, referencePrompt, payload["prompt"])
	assert.Equal(t, 15, payload["duration"])
	assert.Equal(t, int64(0), payload["seed"], "an explicit zero seed must be forwarded")
	assert.Equal(t, "https://cdn.example.com/portrait.png", payload["ref_image_0"])
	assert.Equal(t, "https://cdn.example.com/voice.wav", payload["ref_audio_0"])
	assert.Equal(t, "768p竖", payload["resolution"])
	assert.Equal(t, 1, properties.InputImageCount)

	audioSyncRequest := newMiniMaxRequest(
		8,
		"9:16",
		textContent("The portrait speaks naturally with the supplied voice"),
		imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/portrait.png"),
		audioContent("https://cdn.example.com/voice.wav"),
	)
	audioSyncRequest.AudioSync = pointer(true)
	workflowID, payload, properties, err = buildWorkflowRequest(audioSyncRequest)

	require.NoError(t, err)
	assert.Equal(t, workflowImageAudioSync, workflowID)
	assert.Equal(t, "https://cdn.example.com/portrait.png", payload["ref_image_0"])
	assert.Equal(t, "https://cdn.example.com/voice.wav", payload["ref_audio_0"])
	assert.Equal(t, 8, payload["audio_duration"])
	assert.Equal(t, "768p竖", payload["resolution"])
	assert.NotContains(t, payload, "duration")
	assert.NotContains(t, payload, "prompt")
	assert.Equal(t, 1, properties.InputImageCount)
}

func TestBuildWorkflowRequestRejectsUnsupportedMiniMaxCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		request   *dto.MiniMaxVideoGenerationV2Request
		wantError string
	}{
		{
			name: "2K resolution",
			request: func() *dto.MiniMaxVideoGenerationV2Request {
				request := newMiniMaxRequest(6, "16:9", textContent("A city skyline"))
				request.Resolution = pointer("2K")
				return request
			}(),
			wantError: "supports resolution 768P only",
		},
		{
			name: "reference video",
			request: newMiniMaxRequest(6, "16:9", textContent("Follow the reference"), dto.MiniMaxVideoContentItem{
				Type:     "video_url",
				Role:     miniMaxRoleReferenceVideo,
				VideoURL: &dto.MiniMaxVideoMedia{URL: "https://cdn.example.com/reference.mp4"},
			}),
			wantError: "reference_video is not supported",
		},
		{
			name:      "single first frame",
			request:   newMiniMaxRequest(6, "16:9", textContent("Start here"), imageContent(miniMaxRoleFirstFrame, "https://cdn.example.com/first.png")),
			wantError: "first_frame and last_frame must be provided together",
		},
		{
			name: "square first and last frames",
			request: newMiniMaxRequest(
				6,
				"1:1",
				textContent("Transition between the frames"),
				imageContent(miniMaxRoleFirstFrame, "https://cdn.example.com/first.png"),
				imageContent(miniMaxRoleLastFrame, "https://cdn.example.com/last.png"),
			),
			wantError: "ratio 1:1 is not supported for this input combination",
		},
		{
			name: "square audio synchronized image",
			request: newMiniMaxRequest(
				6,
				"1:1",
				textContent("Speak with the supplied voice"),
				imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/portrait.png"),
				audioContent("https://cdn.example.com/voice.wav"),
			),
			wantError: "ratio 1:1 is not supported for this input combination",
		},
		{
			name:      "adaptive reference ratio",
			request:   newMiniMaxRequest(6, "adaptive", textContent("Animate this"), imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/reference.png")),
			wantError: "ratio must be explicit",
		},
		{
			name: "omitted reference ratio",
			request: func() *dto.MiniMaxVideoGenerationV2Request {
				request := newMiniMaxRequest(6, "16:9", textContent("Animate this"), imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/reference.png"))
				request.Ratio = nil
				return request
			}(),
			wantError: "ratio must be explicit",
		},
		{
			name:      "unsupported ratio",
			request:   newMiniMaxRequest(6, "4:3", textContent("A city skyline")),
			wantError: "ratio is not supported at 768P",
		},
		{
			name:      "duration above billing boundary",
			request:   newMiniMaxRequest(16, "16:9", textContent("A city skyline")),
			wantError: "between 4 and 15",
		},
		{
			name:      "reference audio without image",
			request:   newMiniMaxRequest(6, "16:9", textContent("Follow the voice"), audioContent("https://cdn.example.com/voice.wav")),
			wantError: "requires at least one reference_image",
		},
		{
			name: "audio sync with multiple images",
			request: func() *dto.MiniMaxVideoGenerationV2Request {
				request := newMiniMaxRequest(
					6,
					"16:9",
					textContent("Follow the voice"),
					imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/one.png"),
					imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/two.png"),
					audioContent("https://cdn.example.com/voice.wav"),
				)
				request.AudioSync = pointer(true)
				return request
			}(),
			wantError: "audio_sync=true requires exactly one reference_image and one reference_audio",
		},
		{
			name: "seed with text to video",
			request: func() *dto.MiniMaxVideoGenerationV2Request {
				request := newMiniMaxRequest(6, "16:9", textContent("A city skyline"))
				request.Seed = pointer(int64(42))
				return request
			}(),
			wantError: "seed is supported only for reference-image generation",
		},
		{
			name: "AIGC watermark",
			request: func() *dto.MiniMaxVideoGenerationV2Request {
				request := newMiniMaxRequest(6, "16:9", textContent("A city skyline"))
				request.AIGCWatermark = pointer(true)
				return request
			}(),
			wantError: "aigc_watermark=true is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := buildWorkflowRequest(test.request)
			require.ErrorContains(t, err, test.wantError)
			assert.NotContains(t, strings.ToLower(err.Error()), "autodl")
		})
	}
}

func TestBuildWorkflowRequestBoundsReferenceMediaCounts(t *testing.T) {
	images := []dto.MiniMaxVideoContentItem{textContent("Animate the references")}
	for range 10 {
		images = append(images, imageContent(miniMaxRoleReferenceImage, "https://cdn.example.com/ref.png"))
	}
	_, _, _, err := buildWorkflowRequest(newMiniMaxRequest(6, "16:9", images...))
	require.ErrorContains(t, err, "at most 9 reference images")

	audios := []dto.MiniMaxVideoContentItem{textContent("Animate the references")}
	for range 4 {
		audios = append(audios, audioContent("https://cdn.example.com/ref.wav"))
	}
	_, _, _, err = buildWorkflowRequest(newMiniMaxRequest(6, "16:9", audios...))
	require.ErrorContains(t, err, "at most 3 reference audio files")
}

func TestMiniMaxVideoRequestBoundsDecodedContentItems(t *testing.T) {
	body := `{"model":"MiniMax-H3","content":[` +
		strings.TrimSuffix(strings.Repeat(`{"type":"text","text":"x"},`, dto.MaxMiniMaxVideoContentItems+1), ",") +
		`],"resolution":"768P","duration":4,"ratio":"16:9"}`
	var request dto.MiniMaxVideoGenerationV2Request
	err := common.Unmarshal([]byte(body), &request)
	require.ErrorContains(t, err, "must not contain more than")
}

func TestTaskAdaptorBuildsAutoDLURLAndRawAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://autodl.example/",
			ApiKey:         "autodl-token",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{workflowID: workflowTextToVideo}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://autodl.example/api/v1/comfyui/comfyui_workflow/minimax_h3_lightx2v_no_pic", requestURL)

	req := httptest.NewRequest(http.MethodPost, requestURL, strings.NewReader("{}"))
	require.NoError(t, adaptor.BuildRequestHeader(c, req, info))
	assert.Equal(t, "autodl-token", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestTaskAdaptorPreflightRejectsLocalDispatchConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name      string
		baseURL   string
		proxy     string
		wantError string
	}{
		{name: "invalid base URL", baseURL: "http://autodl.example", wantError: "absolute HTTPS"},
		{name: "invalid proxy URL", baseURL: "https://autodl.example", proxy: "://bad-proxy", wantError: "proxy"},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: test.baseURL,
				ApiKey:         "autodl-token",
				ChannelSetting: dto.ChannelSettings{Proxy: test.proxy},
			}}
			adaptor := &TaskAdaptor{workflowID: workflowTextToVideo}
			adaptor.Init(info)

			err := adaptor.PreflightDispatch(c, info)

			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestTaskAdaptorFetchesAutoDLWorkflowResult(t *testing.T) {
	originalTLSInsecureSkipVerify := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	service.InitHttpClient()
	t.Cleanup(func() {
		common.TLSInsecureSkipVerify = originalTLSInsecureSkipVerify
		service.InitHttpClient()
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/api/v1/comfyui/comfyui_workflow/result/upstream-task", req.URL.Path)
		assert.Equal(t, "autodl-token", req.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"QUEUED"}}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	response, err := adaptor.FetchTask(server.URL+"/", "autodl-token", map[string]any{"task_id": "upstream-task"}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func TestTaskAdaptorDefersClientResponseAndSanitizesPersistedTaskData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"code":"Success","data":{"task_id":"upstream-secret","status":"QUEUED"},"msg":""}`,
		)),
	}

	upstreamTaskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-secret", upstreamTaskID)
	assert.JSONEq(t, `{"code":"Success","data":{"status":"QUEUED"}}`, string(taskData))
	assert.NotContains(t, string(taskData), "upstream-secret")
	assert.Empty(t, recorder.Body.String())
}

func TestTaskAdaptorClassifiesNonSuccessJSONBySubmissionCertainty(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "known rejection without task id", body: `{"code":"InvalidParameter","data":{}}`, wantCode: "generation_submission_rejected"},
		{name: "empty code", body: `{}`, wantCode: "invalid_upstream_response"},
		{name: "unknown code", body: `{"code":"Error","data":{}}`, wantCode: "invalid_upstream_response"},
		{name: "task id makes outcome ambiguous", body: `{"code":"InvalidParameter","data":{"task_id":"possibly-created"}}`, wantCode: "invalid_upstream_response"},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}
			_, _, taskErr := (&TaskAdaptor{}).DoResponse(nil, response, nil)
			require.NotNil(t, taskErr)
			assert.Equal(t, test.wantCode, taskErr.Code)
		})
	}
}

func TestTaskAdaptorMapsAutoDLTaskStatuses(t *testing.T) {
	adaptor := &TaskAdaptor{}

	queued, err := adaptor.ParseTaskResult([]byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"QUEUED"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, queued.Status)
	assert.Equal(t, taskcommon.ProgressQueued, queued.Progress)

	running, err := adaptor.ParseTaskResult([]byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"RUNNING"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, running.Status)
	assert.Equal(t, taskcommon.ProgressInProgress, running.Progress)

	succeeded, err := adaptor.ParseTaskResult([]byte(`{
		"code":"Success",
		"data":{
			"task_id":"upstream-task",
			"status":"SUCCESS",
			"results":[
				{"url":"https://cdn.example.com/preview.png","type":"image"},
				{"url":"https://cdn.example.com/result.mp4","type":"video"}
			]
		}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, succeeded.Status)
	assert.Equal(t, taskcommon.ProgressComplete, succeeded.Progress)
	assert.Equal(t, "https://cdn.example.com/result.mp4", succeeded.Url)

	failed, err := adaptor.ParseTaskResult([]byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"FAILED","error":"generation failed"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failed.Status)
	assert.Equal(t, taskcommon.ProgressComplete, failed.Progress)
	assert.Equal(t, "Generation task failed", failed.Reason)

	cancelled, err := adaptor.ParseTaskResult([]byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"CANCELLED"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, cancelled.Status)
	assert.Equal(t, taskcommon.ProgressComplete, cancelled.Progress)
	assert.Equal(t, "Generation task cancelled", cancelled.Reason)
}

func TestTaskAdaptorRejectsSuccessfulResultWithoutVideoMetadata(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"code":"Success",
		"data":{
			"task_id":"upstream-task",
			"status":"SUCCESS",
			"results":[{"url":"https://cdn.example.com/preview.png","type":"image"}]
		}
	}`))

	require.ErrorContains(t, err, "without a video result")
	assert.Nil(t, result)
}

func TestTaskAdaptorSanitizesPolledTaskData(t *testing.T) {
	adaptor := &TaskAdaptor{}
	raw := []byte(`{
		"code":"Success",
		"request_id":"provider-request",
		"data":{
			"task_id":"upstream-secret",
			"status":"SUCCESS",
			"results":[{"url":"https://cdn.example.com/result.mp4","type":"video"}]
		}
	}`)

	stored := adaptor.SanitizeTaskResult(raw)

	assert.NotContains(t, string(stored), "upstream-secret")
	assert.NotContains(t, string(stored), "provider-request")
	assert.Contains(t, string(stored), "https://cdn.example.com/result.mp4")
}

func TestTaskAdaptorConvertsTaskToMiniMaxVideoV2Response(t *testing.T) {
	adaptor := &TaskAdaptor{}
	t.Run("durable submission checkpoints are queued", func(t *testing.T) {
		for _, status := range []model.TaskStatus{model.TaskStatusReserving, model.TaskStatusCheckpointPending} {
			task := &model.Task{
				SubmitTime: 100,
				TaskID:     "task_checkpoint",
				Action:     constant.TaskActionVideoGenerationV2,
				Status:     status,
				Properties: model.Properties{OriginModelName: "MiniMax-H3"},
			}

			data, err := adaptor.ConvertToMiniMaxVideoV2(task)
			require.NoError(t, err)
			var response dto.MiniMaxVideoGenerationV2QueryResponse
			require.NoError(t, common.Unmarshal(data, &response))
			assert.Equal(t, "queued", response.Task.Status)
		}
	})

	t.Run("successful task", func(t *testing.T) {
		task := &model.Task{
			CreatedAt: 100,
			UpdatedAt: 200,
			TaskID:    "task_public",
			Action:    constant.TaskActionVideoGenerationV2,
			Status:    model.TaskStatusSuccess,
			Properties: model.Properties{
				OriginModelName: "MiniMax-H3",
				Video: &relaycommon.TaskVideoProperties{
					Resolution:      "768P",
					Duration:        12,
					Ratio:           "16:9",
					InputImageCount: 2,
				},
			},
			PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example.com/result.mp4"},
		}

		data, err := adaptor.ConvertToMiniMaxVideoV2(task)
		require.NoError(t, err)
		var response dto.MiniMaxVideoGenerationV2QueryResponse
		require.NoError(t, common.Unmarshal(data, &response))
		assert.Equal(t, "task_public", response.Task.ID)
		assert.Equal(t, "MiniMax-H3", response.Task.Model)
		assert.Equal(t, "succeeded", response.Task.Status)
		assert.Equal(t, int64(100), response.Task.CreatedAt)
		assert.Equal(t, int64(200), response.Task.UpdatedAt)
		assert.Equal(t, "768P", response.Task.Resolution)
		assert.Equal(t, 12, response.Task.Duration)
		assert.Equal(t, "16:9", response.Task.Ratio)
		assert.Equal(t, "generation", response.Task.TaskType)
		assert.Equal(t, "video", response.Task.Modality)
		require.NotNil(t, response.Task.Content)
		assert.Equal(t, taskcommon.BuildProxyURL(task.TaskID), response.Task.Content.URL)
		assert.NotContains(t, response.Task.Content.URL, "cdn.example.com")
		require.NotNil(t, response.Task.Usage)
		assert.Equal(t, 12, response.Task.Usage.TotalSeconds)
		assert.Equal(t, 12, response.Task.Usage.OutputSeconds)
		assert.Equal(t, 2, response.Task.Usage.InputImageCount)
	})

	t.Run("failed task hides provider details", func(t *testing.T) {
		task := &model.Task{
			SubmitTime: 100,
			TaskID:     "task_failed",
			Action:     constant.TaskActionVideoGenerationV2,
			Status:     model.TaskStatusFailure,
			FailReason: "provider.example rejected the prompt",
			Properties: model.Properties{OriginModelName: "MiniMax-H3"},
		}

		data, err := adaptor.ConvertToMiniMaxVideoV2(task)
		require.NoError(t, err)
		var response dto.MiniMaxVideoGenerationV2QueryResponse
		require.NoError(t, common.Unmarshal(data, &response))
		assert.Equal(t, "failed", response.Task.Status)
		require.NotNil(t, response.Task.Error)
		assert.Equal(t, "generation_failed", response.Task.Error.Code)
		assert.Equal(t, "Video generation failed", response.Task.Error.Message)
		assert.NotContains(t, string(data), "provider.example")
		assert.Nil(t, response.Task.Content)
		assert.Nil(t, response.Task.Usage)
	})

	t.Run("historical provider failure is redacted", func(t *testing.T) {
		task := &model.Task{
			SubmitTime: 100,
			TaskID:     "task_failed_legacy",
			Action:     constant.TaskActionVideoGenerationV2,
			Status:     model.TaskStatusFailure,
			FailReason: "AutoDL task failed with internal routing detail",
			Properties: model.Properties{OriginModelName: "MiniMax-H3"},
		}

		data, err := adaptor.ConvertToMiniMaxVideoV2(task)
		require.NoError(t, err)
		var response dto.MiniMaxVideoGenerationV2QueryResponse
		require.NoError(t, common.Unmarshal(data, &response))
		require.NotNil(t, response.Task.Error)
		assert.Equal(t, "Video generation failed", response.Task.Error.Message)
		assert.NotContains(t, strings.ToLower(string(data)), "autodl")
	})

	t.Run("cancelled task", func(t *testing.T) {
		task := &model.Task{
			SubmitTime: 100,
			TaskID:     "task_cancelled",
			Action:     constant.TaskActionVideoGenerationV2,
			Status:     model.TaskStatusFailure,
			FailReason: "AutoDL task cancelled",
			Data:       []byte(`{"code":"Success","data":{"status":"CANCELLED"}}`),
			Properties: model.Properties{OriginModelName: "MiniMax-H3"},
		}

		data, err := adaptor.ConvertToMiniMaxVideoV2(task)
		require.NoError(t, err)
		var response dto.MiniMaxVideoGenerationV2QueryResponse
		require.NoError(t, common.Unmarshal(data, &response))
		assert.Equal(t, "cancelled", response.Task.Status)
		assert.Nil(t, response.Task.Error)
	})
}
