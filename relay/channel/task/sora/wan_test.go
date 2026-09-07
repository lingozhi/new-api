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

func TestUnifiedWanWorkflowMappingAndBilling(t *testing.T) {
	for _, tc := range []struct{ name, fields, target string }{
		{"default", ``, "wan3.0-video"},
		{"fast references", `,"speed":"fast","reference_images":[{"url":"https://example.com/a.jpg","role":"reference_image"}]`, "wan3.0-video-prime"},
		{"reference conversion", `,"mode":"reference","reference_images":[{"url":"https://example.com/a.jpg"}],"reference_videos":[{"url":"https://example.com/a.mp4"}],"reference_audios":[{"url":"https://example.com/a.mp3"}]`, "wan3.0-prime-r2v"},
		{"auto frames", `,"first_frame":"https://example.com/a.jpg","last_frame":"https://example.com/b.jpg"`, "wan3.0-i2v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, info := newWanContext(t, "wan3.0", `{"model":"wan3.0","prompt":"test","seconds":"2","resolution":"480p","prompt_extend":false`+tc.fields+`}`)
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			// The common task pipeline resets this after validation; mapping must survive.
			info.UpstreamModelName = "wan3.0"
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.DecodeJson(body, &payload))
			assert.Equal(t, tc.target, payload["model"])
			assert.Equal(t, tc.target, info.UpstreamModelName)
			assert.Equal(t, "wan3.0", info.OriginModelName)
			assert.Equal(t, false, payload["prompt_extend"])
			assert.Equal(t, "16:9", payload["aspect_ratio"])
			assert.Equal(t, "480P", payload["resolution"])
			assert.NotContains(t, payload, "mode")
			assert.NotContains(t, payload, "speed")
			assert.NotContains(t, payload, "first_frame")
			assert.NotContains(t, payload, "last_frame")
			ratios := adaptor.EstimateBilling(c, info)
			assert.InDelta(t, .5, .3*ratios["seconds"]*ratios["resolution"], 1e-10)
			assert.Equal(t, "wan-unified", info.TaskRelayInfo.Video.Provider)
			if tc.target == "wan3.0-prime-r2v" {
				assert.NotContains(t, payload, "reference_images")
				media := payload["media"].([]any)
				require.Len(t, media, 3)
				assert.Equal(t, "audio", media[2].(map[string]any)["type"])
			}
			if tc.target == "wan3.0-i2v" {
				assert.Len(t, payload["media"], 2)
			}
		})
	}
}

func TestUnifiedWanRejectsAmbiguousOrUnsupportedInputs(t *testing.T) {
	for _, fields := range []string{
		`,"mode":"invalid"`, `,"speed":"turbo"`, `,"mode":"reference","speed":"standard"`,
		`,"mode":"reference"`, `,"mode":"frames","speed":"fast"`,
		`,"first_frame":"https://example.com/a.jpg"`,
		`,"mode":"general","first_frame":"https://example.com/a.jpg","last_frame":"https://example.com/b.jpg"`,
		`,"first_frame":"https://example.com/a.jpg","last_frame":"https://example.com/b.jpg","reference_images":[{"url":"https://example.com/c.jpg"}]`,
		`,"media":[]`, `,"stream":false`, `,"prompt_extend":"false"`, `,"mode":null`,
		`,"seconds":"9999999999999999999"`, `,"duration":31`, `,"resolution":"4K"`, `,"n":2`,
		`,"ratio":"adaptive"`, `,"ratio":"9:16","aspect_ratio":"16:9"`,
		`,"mode":"reference","reference_images":[{"url":"https://example.com/a.jpg","role":"first_frame"}]`,
		`,"mode":"reference","reference_videos":[{"url":"https://example.com/a.mp4","duration":5}]`,
		`,"reference_videos":[{"url":"https://example.com/a.mp4","duration":1e100}]`,
		`,"reference_images":[{"url":"http://example.com/a.jpg"}]`,
	} {
		t.Run(fields, func(t *testing.T) {
			c, info := newWanContext(t, "wan3.0", `{"prompt":"test"`+fields+`}`)
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
			_, exists := c.Get("wan_unified_body")
			assert.False(t, exists)
		})
	}
}

func TestUnifiedWanResponseKeepsPublicIdentity(t *testing.T) {
	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusInProgress, Data: []byte(`{"id":"upstream","request_id":"upstream","task_id":"upstream","model":"wan3.0-i2v","status":"processing"}`)}
	task.Properties.OriginModelName = "wan3.0"
	task.Properties.Video = &relaycommon.TaskVideoProperties{Provider: "wan-unified"}
	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	for _, field := range []string{"id", "task_id", "request_id"} {
		assert.Equal(t, "task_public", payload[field])
	}
	assert.Equal(t, "wan3.0", payload["model"])
	assert.Equal(t, "in_progress", payload["status"])
}

func TestUnifiedWanCreationKeepsProviderIDPrivate(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{OriginModelName: "wan3.0", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "wan3.0-i2v"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
	response := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream","model":"wan3.0-i2v","status":"queued"}`))}
	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream", upstreamID)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	for _, key := range []string{"id", "task_id", "request_id"} {
		assert.Equal(t, "task_public", payload[key])
	}
	assert.Equal(t, "wan3.0", payload["model"])
}

func TestUnifiedWanReferenceLimitsBeforeBilling(t *testing.T) {
	for field, count := range map[string]int{"reference_images": 11, "reference_videos": 6, "reference_audios": 6} {
		t.Run(field, func(t *testing.T) {
			entries := make([]map[string]string, count)
			for i := range entries {
				entries[i] = map[string]string{"url": "https://example.com/asset"}
			}
			raw, err := common.Marshal(map[string]any{"model": "wan3.0", "prompt": "test", "mode": "reference", field: entries})
			require.NoError(t, err)
			c, info := newWanContext(t, "wan3.0", string(raw))
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			_, exists := c.Get("wan_unified_body")
			assert.False(t, exists)
		})
	}
}
