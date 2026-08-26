package autodl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexTTSAudioSpeechAction = "audioSpeech"

func validateIndexTTSSpeechRequest(t *testing.T, body string) (*TaskAdaptor, *relaycommon.RelayInfo, *dto.TaskError) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Idempotency-Key", "indextts2-adaptor-test")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	previousMaterialize := materializeIndexTTSAudio
	materializeIndexTTSAudio = func(_ context.Context, source string, currentBytes int) (string, int, error) {
		return source, currentBytes, nil
	}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	materializeIndexTTSAudio = previousMaterialize
	return adaptor, info, taskErr
}

func indexTTSPayload(t *testing.T, adaptor *TaskAdaptor, info *relaycommon.RelayInfo) map[string]any {
	t.Helper()
	body, err := adaptor.BuildRequestBody(nil, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	return payload
}

func TestIndexTTS2MapsOpenAISpeechRequestToWorkflowPayload(t *testing.T) {
	adaptor, info, taskErr := validateIndexTTSSpeechRequest(t, `{
		"model":"indextts2-v1",
		"input":"你好，欢迎使用 IndexTTS2。",
		"voice":"https://media.example.com/speaker.wav?signature=voice",
		"response_format":"wav",
		"speed":1,
		"metadata":{
			"emotion_vector":[0.2,0,0.1,0,0,0,0,0.3],
			"emotion_random":false
		}
	}`)
	require.Nil(t, taskErr)

	assert.Equal(t, indexTTSAudioSpeechAction, info.Action)
	assert.Equal(t, "indextts2-v1", adaptor.workflowID)
	assert.Nil(t, adaptor.EstimateBilling(nil, info), "IndexTTS2 is billed per task, not by duration")

	payload := indexTTSPayload(t, adaptor, info)
	assert.Equal(t, "你好，欢迎使用 IndexTTS2。", payload["prompt_text"])
	assert.Equal(t, "https://media.example.com/speaker.wav?signature=voice", payload["prompt_simple"])
	assert.Equal(t, float64(0.2), payload["emo_happy"])
	assert.Equal(t, float64(0), payload["emo_angry"])
	assert.Equal(t, float64(0.1), payload["emo_sad"])
	assert.Equal(t, float64(0), payload["emo_afraid"])
	assert.Equal(t, float64(0), payload["emo_disgusted"])
	assert.Equal(t, float64(0), payload["emo_melancholic"])
	assert.Equal(t, "0", payload["emo_surprised"])
	assert.Equal(t, float64(0.3), payload["emo_calm"])
	assert.Equal(t, false, payload["emo_random"], "an explicit false value must be preserved")
	assert.NotEmpty(t, payload["emo_control_method"])
	assert.NotContains(t, payload, "model")
	assert.NotContains(t, payload, "response_format")
}

func TestIndexTTS2MapsCompleteModelParametersToWorkflowPayload(t *testing.T) {
	adaptor, info, taskErr := validateIndexTTSSpeechRequest(t, `{
		"model":"indextts2-v1",
		"emo_sad":0,
		"emo_calm":0.3,
		"emo_angry":0,
		"emo_happy":0.5,
		"emo_afraid":0,
		"emo_random":false,
		"prompt_text":"你好，这是一段测试文本",
		"emo_disgusted":0,
		"emo_ref_audio":"https://media.example.com/emotion.mp3",
		"emo_surprised":"0",
		"prompt_simple":"https://media.example.com/speaker.wav",
		"emo_melancholic":0,
		"emo_control_method":"与音色参考音频相同"
	}`)
	require.Nil(t, taskErr)

	payload := indexTTSPayload(t, adaptor, info)
	assert.Equal(t, "你好，这是一段测试文本", payload["prompt_text"])
	assert.Equal(t, "https://media.example.com/speaker.wav", payload["prompt_simple"])
	assert.Equal(t, "与音色参考音频相同", payload["emo_control_method"])
	assert.Equal(t, "https://media.example.com/emotion.mp3", payload["emo_ref_audio"])
	assert.Equal(t, float64(0), payload["emo_sad"])
	assert.Equal(t, float64(0.3), payload["emo_calm"])
	assert.Equal(t, float64(0), payload["emo_angry"])
	assert.Equal(t, float64(0.5), payload["emo_happy"])
	assert.Equal(t, float64(0), payload["emo_afraid"])
	assert.Equal(t, float64(0), payload["emo_disgusted"])
	assert.Equal(t, float64(0), payload["emo_melancholic"])
	assert.Equal(t, "0", payload["emo_surprised"])
	assert.Equal(t, false, payload["emo_random"])
}

func TestIndexTTS2MapsEmotionReferenceAudio(t *testing.T) {
	adaptor, info, taskErr := validateIndexTTSSpeechRequest(t, `{
		"model":"indextts2-v1",
		"input":"带有参考情绪的语音",
		"voice":"data:audio/wav;base64,UklGRg==",
		"metadata":{"emotion_audio":"https://media.example.com/emotion.mp3","emotion_random":false}
	}`)
	require.Nil(t, taskErr)

	payload := indexTTSPayload(t, adaptor, info)
	assert.Equal(t, "data:audio/wav;base64,UklGRg==", payload["prompt_simple"])
	assert.Equal(t, "https://media.example.com/emotion.mp3", payload["emo_ref_audio"])
	assert.Equal(t, "与音色参考音频相同", payload["emo_control_method"])
}

func TestIndexTTS2DefaultsToVoiceReferenceEmotion(t *testing.T) {
	adaptor, info, taskErr := validateIndexTTSSpeechRequest(t, `{
		"model":"indextts2-v1",
		"input":"使用音色参考的情绪",
		"voice":"https://media.example.com/speaker.mp3"
	}`)
	require.Nil(t, taskErr)

	payload := indexTTSPayload(t, adaptor, info)
	assert.Equal(t, "与音色参考音频相同", payload["emo_control_method"])
	assert.NotContains(t, payload, "emo_ref_audio")
}

func TestIndexTTS2ValidatesOpenAISpeechCompatibility(t *testing.T) {
	boundaryText := strings.Repeat("声", 2048)
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name: "one character input",
			body: `{"model":"indextts2-v1","input":"声","voice":"https://media.example.com/speaker.wav"}`,
		},
		{
			name: "complete model text and audio parameters",
			body: `{"model":"indextts2-v1","prompt_text":"声","prompt_simple":"https://media.example.com/speaker.wav","emo_control_method":"与音色参考音频相同"}`,
		},
		{
			name: "2048 unicode character input and explicit WAV",
			body: `{"model":"indextts2-v1","input":"` + boundaryText + `","voice":"https://media.example.com/speaker.wav","response_format":"wav","speed":1}`,
		},
		{
			name:      "wrong model",
			body:      `{"model":"tts-1","input":"hello","voice":"https://media.example.com/speaker.wav"}`,
			wantError: "model",
		},
		{
			name:      "blank input",
			body:      `{"model":"indextts2-v1","input":"   ","voice":"https://media.example.com/speaker.wav"}`,
			wantError: "input",
		},
		{
			name:      "input above provider boundary",
			body:      `{"model":"indextts2-v1","input":"` + boundaryText + `声","voice":"https://media.example.com/speaker.wav"}`,
			wantError: "2048",
		},
		{
			name:      "named OpenAI voice cannot be resolved",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"alloy"}`,
			wantError: "voice",
		},
		{
			name:      "MP3 response would mislabel WAV bytes",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/speaker.wav","response_format":"mp3"}`,
			wantError: "response_format",
		},
		{
			name:      "unsupported speed",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/speaker.wav","speed":0.9}`,
			wantError: "speed",
		},
		{
			name:      "unsupported instructions",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/speaker.wav","instructions":"whisper"}`,
			wantError: "instructions",
		},
		{
			name:      "unsupported streaming",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/speaker.wav","stream_format":"sse"}`,
			wantError: "stream",
		},
		{
			name:      "conflicting text aliases",
			body:      `{"model":"indextts2-v1","input":"one","prompt_text":"two","voice":"https://media.example.com/speaker.wav"}`,
			wantError: "match",
		},
		{
			name:      "conflicting audio aliases",
			body:      `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/one.wav","prompt_simple":"https://media.example.com/two.wav"}`,
			wantError: "match",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, taskErr := validateIndexTTSSpeechRequest(t, test.body)
			if test.wantError == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, strings.ToLower(taskErr.Message), strings.ToLower(test.wantError))
			assert.NotContains(t, strings.ToLower(taskErr.Message), "autodl")
		})
	}
}

func TestIndexTTS2ValidatesCompleteModelEmotionParameters(t *testing.T) {
	tests := []struct {
		name      string
		fields    string
		wantError string
	}{
		{name: "provider upper boundary", fields: `,"emo_happy":1.4`},
		{name: "numeric surprised zero", fields: `,"emo_surprised":0`},
		{name: "string surprised zero", fields: `,"emo_surprised":"0"`},
		{name: "negative emotion", fields: `,"emo_afraid":-0.01`, wantError: "emo_afraid"},
		{name: "emotion above range", fields: `,"emo_calm":1.4001`, wantError: "emo_calm"},
		{name: "unsupported surprise", fields: `,"emo_surprised":0.1`, wantError: "emo_surprised"},
		{name: "unsupported surprise string", fields: `,"emo_surprised":"0.1"`, wantError: "emo_surprised"},
		{name: "unsupported control method", fields: `,"emo_control_method":"unsupported"`, wantError: "emo_control_method"},
		{name: "direct fields and legacy metadata conflict", fields: `,"emo_random":false,"metadata":{"emotion_random":false}`, wantError: "cannot be combined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"model":"indextts2-v1","prompt_text":"hello","prompt_simple":"https://media.example.com/speaker.wav"` + test.fields + `}`
			_, _, taskErr := validateIndexTTSSpeechRequest(t, body)
			if test.wantError == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, strings.ToLower(taskErr.Message), strings.ToLower(test.wantError))
		})
	}
}

func TestIndexTTS2ValidatesEmotionMetadata(t *testing.T) {
	tests := []struct {
		name      string
		metadata  string
		wantError string
	}{
		{name: "zero vector", metadata: `{"emotion_vector":[0,0,0,0,0,0,0,0]}`},
		{name: "provider upper boundary", metadata: `{"emotion_vector":[1.4,0,0,0,0,0,0,0]}`},
		{name: "random only", metadata: `{"emotion_random":true}`},
		{name: "wrong vector length", metadata: `{"emotion_vector":[0,0,0]}`, wantError: "8"},
		{name: "negative component", metadata: `{"emotion_vector":[-0.01,0,0,0,0,0,0,0]}`, wantError: "range"},
		{name: "above provider range", metadata: `{"emotion_vector":[1.4001,0,0,0,0,0,0,0]}`, wantError: "range"},
		{name: "surprised is not exposed upstream yet", metadata: `{"emotion_vector":[0,0,0,0,0,0,0.1,0]}`, wantError: "surprised"},
		{name: "reference audio and vector are exclusive", metadata: `{"emotion_audio":"https://media.example.com/emotion.wav","emotion_vector":[0,0,0,0,0,0,0,0]}`, wantError: "exclusive"},
		{name: "random and vector are exclusive", metadata: `{"emotion_random":true,"emotion_vector":[0,0,0,0,0,0,0,0]}`, wantError: "exclusive"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"model":"indextts2-v1","input":"hello","voice":"https://media.example.com/speaker.wav","metadata":` + test.metadata + `}`
			_, _, taskErr := validateIndexTTSSpeechRequest(t, body)
			if test.wantError == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, strings.ToLower(taskErr.Message), strings.ToLower(test.wantError))
		})
	}
}

func TestIndexTTS2AppliesAudioMediaSafetyToVoiceAndEmotion(t *testing.T) {
	tests := []struct {
		name      string
		voice     string
		metadata  string
		wantError string
	}{
		{name: "WAV data URI", voice: "data:audio/wav;base64,UklGRg=="},
		{name: "MP3 data URI", voice: "data:audio/mpeg;base64,SUQz"},
		{name: "HTTP voice", voice: "http://media.example.com/speaker.wav", wantError: "HTTPS"},
		{name: "private voice", voice: "https://127.0.0.1/speaker.wav", wantError: "public IP"},
		{name: "wrong voice MIME", voice: "data:image/png;base64,iVBORw0KGgo=", wantError: "not allowed"},
		{name: "private emotion audio", voice: "https://media.example.com/speaker.wav", metadata: `,"metadata":{"emotion_audio":"https://169.254.169.254/emotion.wav"}`, wantError: "public IP"},
		{name: "wrong emotion MIME", voice: "https://media.example.com/speaker.wav", metadata: `,"metadata":{"emotion_audio":"data:image/png;base64,iVBORw0KGgo="}`, wantError: "not allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"model":"indextts2-v1","input":"hello","voice":"` + test.voice + `"` + test.metadata + `}`
			_, _, taskErr := validateIndexTTSSpeechRequest(t, body)
			if test.wantError == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, taskErr.Message, test.wantError)
		})
	}
}

type actionTaskResultParser interface {
	ParseTaskResultForAction(respBody []byte, action string) (*relaycommon.TaskInfo, error)
}

func TestIndexTTS2TaskResultParserSelectsMediaForTaskAction(t *testing.T) {
	parser, ok := any(&TaskAdaptor{}).(actionTaskResultParser)
	require.True(t, ok, "AutoDL polling must parse against the persisted task action instead of mutable adaptor state")
	if !ok {
		return
	}

	resultBody := []byte(`{
		"code":"Success",
		"data":{
			"task_id":"upstream-task",
			"status":"completed",
			"results":[
				{"url":"https://media.example.com/preview.png","type":"image","file_type":"png"},
				{"url":"https://media.example.com/result.mp4","type":"video","file_type":"mp4"},
				{"url":"https://media.example.com/result.wav?signature=short-lived","type":"audio","file_type":"wav","output_type":"output"}
			]
		}
	}`)

	audio, err := parser.ParseTaskResultForAction(resultBody, indexTTSAudioSpeechAction)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, audio.Status)
	assert.Equal(t, taskcommon.ProgressComplete, audio.Progress)
	assert.Equal(t, "https://media.example.com/result.wav?signature=short-lived", audio.Url)

	video, err := parser.ParseTaskResultForAction(resultBody, "videoGenerationV2")
	require.NoError(t, err)
	assert.Equal(t, "https://media.example.com/result.mp4", video.Url)
}

func TestIndexTTS2TaskResultParserRejectsWrongOrUnsafeMedia(t *testing.T) {
	parser, ok := any(&TaskAdaptor{}).(actionTaskResultParser)
	require.True(t, ok, "AutoDL polling must parse against the persisted task action")
	if !ok {
		return
	}

	tests := []struct {
		name      string
		action    string
		result    string
		wantError string
	}{
		{
			name:      "speech task cannot consume video",
			action:    indexTTSAudioSpeechAction,
			result:    `{"url":"https://media.example.com/result.mp4","type":"video","file_type":"mp4"}`,
			wantError: "audio",
		},
		{
			name:      "video task cannot consume audio",
			action:    "videoGenerationV2",
			result:    `{"url":"https://media.example.com/result.wav","type":"audio","file_type":"wav"}`,
			wantError: "video",
		},
		{
			name:      "speech output must be HTTPS",
			action:    indexTTSAudioSpeechAction,
			result:    `{"url":"http://media.example.com/result.wav","type":"audio","file_type":"wav"}`,
			wantError: "unsafe",
		},
		{
			name:      "speech output must not target private IP",
			action:    indexTTSAudioSpeechAction,
			result:    `{"url":"https://127.0.0.1/result.wav","type":"audio","file_type":"wav"}`,
			wantError: "unsafe",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"code":"Success","data":{"task_id":"upstream-task","status":"SUCCESS","results":[` + test.result + `]}}`)
			result, err := parser.ParseTaskResultForAction(body, test.action)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(test.wantError))
		})
	}
}

func TestIndexTTS2TaskQueryErrorsRemainRetryable(t *testing.T) {
	parser := &TaskAdaptor{}
	for _, code := range []string{"Unauthorized", "Forbidden", "InvalidParameter"} {
		t.Run(code, func(t *testing.T) {
			result, err := parser.ParseTaskResultForAction([]byte(`{"code":"`+code+`","data":{}}`), indexTTSAudioSpeechAction)
			require.Error(t, err)
			assert.Nil(t, result)
			var temporary interface{ Temporary() bool }
			require.ErrorAs(t, err, &temporary)
			assert.True(t, temporary.Temporary(), "query-level errors do not prove the accepted task failed")
		})
	}
}
