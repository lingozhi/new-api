package autodl

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validIndexTTSWAV() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 38, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0,
		1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0,
	}
}

func validIndexTTSMP3() []byte {
	header := []byte{0xff, 0xfb, 0x90, 0x64}
	frameLength, _, _, ok := indexTTSMP3FrameInfo(header)
	if !ok {
		panic("invalid IndexTTS2 MP3 fixture header")
	}
	frames := make([]byte, frameLength*2)
	copy(frames, header)
	copy(frames[frameLength:], header)
	return frames
}

func validIndexTTSWAVDataURI() string {
	return "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(validIndexTTSWAV())
}

func TestNormalizeIndexTTSAudioPayloadPreservesExternalURLs(t *testing.T) {
	payload := map[string]any{
		indexTTSAudioPayloadVoice:   "https://media.example.com/voice.wav?signature=voice",
		indexTTSAudioPayloadEmotion: "https://media.example.com/emotion.mp3?signature=emotion",
	}

	require.NoError(t, normalizeIndexTTSAudioPayload(payload))
	assert.Equal(t, "https://media.example.com/voice.wav?signature=voice", payload[indexTTSAudioPayloadVoice])
	assert.Equal(t, "https://media.example.com/emotion.mp3?signature=emotion", payload[indexTTSAudioPayloadEmotion])

	err := normalizeIndexTTSAudioPayload(map[string]any{
		indexTTSAudioPayloadVoice: "https://media.example.com/" + strings.Repeat("a", maxIndexTTSAudioURLBytes),
	})
	require.ErrorContains(t, err, "URL is too long")
}

func TestNormalizeIndexTTSAudioPayloadValidatesDataURIBytes(t *testing.T) {
	wavURI := "data:audio/x-wav;base64," + base64.StdEncoding.EncodeToString(validIndexTTSWAV())
	mp3URI := "data:audio/mp3;base64," + base64.StdEncoding.EncodeToString(validIndexTTSMP3())
	payload := map[string]any{
		indexTTSAudioPayloadVoice:   wavURI,
		indexTTSAudioPayloadEmotion: mp3URI,
	}

	require.NoError(t, normalizeIndexTTSAudioPayload(payload))
	assert.Equal(t, validIndexTTSWAVDataURI(), payload[indexTTSAudioPayloadVoice])
	assert.Equal(t, "data:audio/mpeg;base64,"+base64.StdEncoding.EncodeToString(validIndexTTSMP3()), payload[indexTTSAudioPayloadEmotion])

	err := normalizeIndexTTSAudioPayload(map[string]any{
		indexTTSAudioPayloadVoice: "data:audio/wav;base64,UklGRg==",
	})
	require.ErrorContains(t, err, "valid WAV or MP3")
}

func TestIndexTTSMP3ValidationRequiresCompleteConsecutiveFrames(t *testing.T) {
	frames := validIndexTTSMP3()
	assert.True(t, isValidIndexTTSMP3(frames))
	assert.False(t, isValidIndexTTSMP3(frames[:4]))
}

func TestIndexTTSAdaptorForwardsCompactReferenceURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{
		"model":"indextts2-v1",
		"prompt_text":"compact references",
		"prompt_simple":"https://media.example.com/voice.wav?signature=voice",
		"emo_ref_audio":"https://media.example.com/emotion.mp3?signature=emotion",
		"emo_control_method":"与音色参考音频相同"
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Request.Header.Set("Idempotency-Key", "indextts2-reference-test")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	encodedPayload, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	assert.Less(t, len(encodedPayload), 1024)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encodedPayload, &payload))
	assert.Equal(t, "https://media.example.com/voice.wav?signature=voice", payload[indexTTSAudioPayloadVoice])
	assert.Equal(t, "https://media.example.com/emotion.mp3?signature=emotion", payload[indexTTSAudioPayloadEmotion])
}
