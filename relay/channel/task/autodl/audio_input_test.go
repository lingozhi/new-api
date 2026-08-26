package autodl

import (
	"bytes"
	"context"
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

type indexTTSAudioRoundTripFunc func(*http.Request) (*http.Response, error)

func (f indexTTSAudioRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func validIndexTTSWAV() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 38, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0,
		1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0,
	}
}

func installIndexTTSAudioFetch(t *testing.T, contentType string, body []byte) {
	t.Helper()
	previousValidate := validateIndexTTSAudioURL
	previousClient := indexTTSAudioHTTPClient
	validateIndexTTSAudioURL = func(string) error { return nil }
	indexTTSAudioHTTPClient = func() *http.Client {
		return &http.Client{Transport: indexTTSAudioRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": []string{contentType}},
				Body:          io.NopCloser(bytes.NewReader(body)),
				ContentLength: int64(len(body)),
				Request:       request,
			}, nil
		})}
	}
	t.Cleanup(func() {
		validateIndexTTSAudioURL = previousValidate
		indexTTSAudioHTTPClient = previousClient
	})
}

func TestFetchIndexTTSAudioReplacesExternalURLWithValidatedDataURI(t *testing.T) {
	wav := validIndexTTSWAV()
	installIndexTTSAudioFetch(t, "application/octet-stream", wav)

	dataURI, total, err := fetchIndexTTSAudioDataURI(context.Background(), "https://media.example.com/voice.wav?signature=secret", 0)
	require.NoError(t, err)
	assert.Equal(t, len(wav), total)
	assert.NotContains(t, dataURI, "media.example.com")
	assert.Equal(t, "data:audio/wav;base64,"+base64.StdEncoding.EncodeToString(wav), dataURI)
}

func TestFetchIndexTTSAudioRejectsFakeDataAndMIMEConflicts(t *testing.T) {
	_, _, err := fetchIndexTTSAudioDataURI(context.Background(), "data:audio/wav;base64,UklGRg==", 0)
	require.ErrorContains(t, err, "valid WAV or MP3")

	installIndexTTSAudioFetch(t, "audio/mpeg", validIndexTTSWAV())
	_, _, err = fetchIndexTTSAudioDataURI(context.Background(), "https://media.example.com/voice.wav", 0)
	require.ErrorContains(t, err, "content type")
}

func TestIndexTTSMP3ValidationRequiresCompleteConsecutiveFrames(t *testing.T) {
	header := []byte{0xff, 0xfb, 0x90, 0x64}
	frameLength, _, _, ok := indexTTSMP3FrameInfo(header)
	require.True(t, ok)
	frames := make([]byte, frameLength*2)
	copy(frames, header)
	copy(frames[frameLength:], header)
	assert.True(t, isValidIndexTTSMP3(frames))
	assert.False(t, isValidIndexTTSMP3(header))
}

func TestIndexTTSAdaptorNeverForwardsReferenceURLsToAutoDL(t *testing.T) {
	previousMaterialize := materializeIndexTTSAudio
	materializeIndexTTSAudio = func(_ context.Context, source string, currentBytes int) (string, int, error) {
		encoded := base64.StdEncoding.EncodeToString([]byte(source))
		return "data:audio/wav;base64," + encoded, currentBytes + len(source), nil
	}
	t.Cleanup(func() { materializeIndexTTSAudio = previousMaterialize })

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{
		"model":"indextts2-v1",
		"input":"secure references",
		"voice":"https://media.example.com/voice.wav?signature=voice",
		"metadata":{"emotion_audio":"https://media.example.com/emotion.mp3?signature=emotion"}
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
	assert.NotContains(t, string(encodedPayload), "media.example.com")
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encodedPayload, &payload))
	assert.True(t, strings.HasPrefix(payload[indexTTSAudioPayloadVoice].(string), "data:audio/wav;base64,"))
	assert.True(t, strings.HasPrefix(payload[indexTTSAudioPayloadEmotion].(string), "data:audio/wav;base64,"))
}
