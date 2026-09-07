package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWanContext(t *testing.T, modelName, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: modelName, ChannelBaseUrl: "https://provider.example"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	return c, info
}

func TestWanVideoBillingMatchesRelayedParameters(t *testing.T) {
	for _, tc := range []struct {
		name, body, resolution string
		duration               int
		cost                   float64
	}{
		{"480p", `{"prompt":"test","seconds":"2","size":"480P","prompt_extend":false}`, "480p", 2, 0.5},
		{"720p aliases", `{"prompt":"test","duration":2,"resolution":"720p"}`, "720p", 2, 0.6},
		{"1080p", `{"prompt":"test","duration":30,"seconds":"30","size":"1080P","resolution":"1080p"}`, "1080p", 30, 10.5},
		{"defaults", `{"prompt":"test"}`, "720p", 5, 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, info := newWanContext(t, "wan3.0-video", tc.body)
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			ratios := adaptor.EstimateBilling(c, info)
			assert.InDelta(t, tc.cost, 0.30*ratios["seconds"]*ratios["resolution"], 1e-10)
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			encoded, err := io.ReadAll(body)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(encoded, &payload))
			assert.Equal(t, float64(tc.duration), payload["duration"])
			assert.Equal(t, strings.ToUpper(tc.resolution), payload["size"])
			assert.Equal(t, payload["size"], payload["resolution"])
			assert.Equal(t, "wan3.0-video", payload["model"])
			assert.Equal(t, tc.duration, info.TaskRelayInfo.Video.Duration)
			if tc.name == "480p" {
				assert.Equal(t, false, payload["prompt_extend"])
			}
		})
	}
}

func TestWanVideoRejectsUnsafeOrConflictingInputs(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"test","duration":0}`,
		`{"prompt":"test","duration":31}`,
		`{"prompt":"test","seconds":"18446744073686646784"}`,
		`{"prompt":"test","seconds":"-1"}`,
		`{"prompt":"test","seconds":"2.5"}`,
		`{"prompt":"test","seconds":"2","duration":10}`,
		`{"prompt":"test","size":"480P","resolution":"1080P"}`,
		`{"prompt":"test","resolution":"4K"}`,
		`{"prompt":"test","n":2}`,
		`{"prompt":"test","n":0}`,
		`{"prompt":"test","ratio":"9:16","aspect_ratio":"16:9"}`,
		`{"prompt":"test","media":[{"type":"reference_image","url":"https://example.com/a.png"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			c, info := newWanContext(t, "wan3.0-video", body)
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
		})
	}
}

func TestWanMediaModelsPreserveInputsAndPricing(t *testing.T) {
	for _, tc := range []struct {
		model, body string
		valid       bool
	}{
		{"wan3.0-video-prime", `{"prompt":"test","reference_images":[{"url":"https://example.com/a.png","role":"first_frame"}]}`, true},
		{"wan3.0-prime-r2v", `{"prompt":"test","media":[{"type":"reference_image","url":"https://example.com/a.png"}]}`, true},
		{"wan3.0-i2v", `{"prompt":"test","media":[{"type":"first_frame","url":"https://example.com/a.png"},{"type":"last_frame","url":"https://example.com/b.png"}]}`, true},
		{"wan3.0-i2v", `{"prompt":"test","media":[{"type":"first_frame","url":"https://example.com/a.png"}]}`, false},
		{"wan3.0-prime-r2v", `{"prompt":"test","media":[{"type":"first_frame","url":"https://example.com/a.png"}]}`, false},
		{"wan3.0-prime-r2v", `{"prompt":"test"}`, false},
	} {
		t.Run(tc.model+tc.body, func(t *testing.T) {
			c, info := newWanContext(t, tc.model, tc.body)
			adaptor := &TaskAdaptor{}
			err := adaptor.ValidateRequestAndSetAction(c, info)
			if !tc.valid {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.InDelta(t, 1.5, .30*adaptor.EstimateBilling(c, info)["seconds"], 1e-10)
			body, buildErr := adaptor.BuildRequestBody(c, info)
			require.NoError(t, buildErr)
			encoded, readErr := io.ReadAll(body)
			require.NoError(t, readErr)
			var before, after map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &before))
			require.NoError(t, common.Unmarshal(encoded, &after))
			assert.Equal(t, before["media"], after["media"])
			assert.Equal(t, before["reference_images"], after["reference_images"])
		})
	}
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"archiving","progress":95}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
}
