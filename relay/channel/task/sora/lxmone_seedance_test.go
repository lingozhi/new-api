package sora

import (
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLxmoneSeedanceNormalizesProtocolAndBilling(t *testing.T) {
	for _, tc := range []struct {
		model, body, upstream, resolution string
		duration                          int
	}{
		{"seedance-2", `{"prompt":"test","seconds":4,"resolution":"4K"}`, "seedance-2-pro", "4k", 4},
		{"seedance-2-fast", `{"prompt":"test","seconds":"10","size":"1080P"}`, "seedance-2-fast", "720p", 10},
		{"seedance-2-mini", `{"prompt":"test","duration":5,"resolution":"1080P"}`, "seedance-2-mini", "720p", 5},
		{"seedance-2.5", `{"prompt":"test","duration":30,"resolution":"4K","prompt_extend":false}`, "seedance-2.5-pro", "1080p", 30},
		{"seedance-2", `{"prompt":"test"}`, "seedance-2-pro", "720p", 5},
	} {
		t.Run(tc.model+tc.resolution, func(t *testing.T) {
			c, info := newWanContext(t, tc.model, tc.body)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			info.UpstreamModelName = tc.upstream
			info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{tc.upstream: {tc.resolution: 1.25}}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			endpoint, err := adaptor.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://lxmone.xyz/v1/videos", endpoint)
			assert.Equal(t, map[string]float64{"seconds": float64(tc.duration), "resolution": 1.25}, adaptor.EstimateBilling(c, info))
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			encoded, err := io.ReadAll(body)
			require.NoError(t, err)
			var actual map[string]any
			require.NoError(t, common.Unmarshal(encoded, &actual))
			assert.Equal(t, tc.upstream, actual["model"])
			assert.Equal(t, float64(tc.duration), actual["duration"])
			assert.Equal(t, strings.ToUpper(tc.resolution), actual["resolution"])
			assert.Equal(t, actual["resolution"], actual["size"])
			assert.Equal(t, "16:9", actual["aspect_ratio"])
			assert.Equal(t, "lxmone-seedance", info.TaskRelayInfo.Video.Provider)
			if tc.model == "seedance-2.5" {
				assert.Equal(t, false, actual["prompt_extend"])
			}
		})
	}
}

func TestLxmoneSeedanceRejectsInvalidRequestsBeforePricing(t *testing.T) {
	for _, tc := range []struct{ model, body string }{
		{"seedance-2", `{"prompt":"x","seconds":3}`},
		{"seedance-2", `{"prompt":"x","seconds":16}`},
		{"seedance-2.5", `{"prompt":"x","seconds":31}`},
		{"seedance-2-fast", `{"prompt":"x","seconds":4}`},
		{"seedance-2-mini", `{"prompt":"x","seconds":6}`},
		{"seedance-2", `{"prompt":"x","seconds":"18446744073686646784"}`},
		{"seedance-2", `{"prompt":"x","seconds":4.5}`},
		{"seedance-2", `{"prompt":"x","seconds":null}`},
		{"seedance-2", `{"prompt":"x","seconds":4,"duration":5}`},
		{"seedance-2", `{"prompt":"x","size":"480p","resolution":"720p"}`},
		{"seedance-2", `{"prompt":"x","stream":false}`},
		{"seedance-2", `{"prompt":"x","n":1}`},
		{"seedance-2", `{"prompt":"x","webhook_url":"https://example.com"}`},
		{"seedance-2", `{"prompt":"x","image_end":"https://example.com/end.png"}`},
		{"seedance-2", `{"prompt":"x","image":"https://example.com/start.png","reference_images":[{"url":"https://example.com/i.png"}]}`},
		{"seedance-2", `{"prompt":"x","reference_audios":[{"url":"https://example.com/a.mp3"}]}`},
		{"seedance-2-mini", `{"prompt":"x","resolution":"4k"}`},
	} {
		t.Run(tc.model+tc.body, func(t *testing.T) {
			c, info := newWanContext(t, tc.model, tc.body)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.NotEqual(t, "model_price_error", taskErr.Code)
		})
	}
}

func TestLxmoneSeedanceRequiresOwnResolutionPricing(t *testing.T) {
	for _, ratio := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		c, info := newWanContext(t, "seedance-2.5", `{"prompt":"x"}`)
		info.ChannelBaseUrl = "https://lxmone.xyz"
		info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2.5-pro": {"720p": ratio}}
		err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
		require.NotNil(t, err)
		assert.Equal(t, "model_price_error", err.Code)
	}
}

func TestSeedanceProvidersKeepSeparateProtocols(t *testing.T) {
	c, info := newWanContext(t, "seedance-2.5", `{"model":"seedance-2.5","prompt":"x","duration":4,"resolution":"480p"}`)
	info.ChannelBaseUrl = "https://argolink.example"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	endpoint, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://argolink.example/v1/videos/generations", endpoint)
	ratios := adaptor.EstimateBilling(c, info)
	assert.Equal(t, float64(2), ratios["preconsume_buffer"])
	assert.InDelta(t, .077/.17, ratios["resolution"], 1e-12)
	assert.Empty(t, info.TaskRelayInfo.Video.Provider)

	task := &model.Task{TaskID: "public-id", Status: model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "seedance-2.5", Video: &relaycommon.TaskVideoProperties{Provider: "lxmone-seedance"}},
		Data:       []byte(`{"id":"upstream-id","task_id":"upstream-id","model":"seedance-2.5-pro","status":"completed"}`)}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(body, &result))
	assert.Equal(t, "seedance-2.5", result["model"])
	assert.Equal(t, "public-id", result["id"])
	assert.Equal(t, "public-id", result["task_id"])
	assert.Equal(t, "completed", result["status"])
}

func TestLxmoneSeedancePreservesReferenceMediaAndPublicNames(t *testing.T) {
	c, recorder, info := newArgolinkSeedanceContext(t, `{"prompt":"test","image":"https://example.com/first.png","image_end":"https://example.com/last.png","prompt_extend":false}`)
	info.ChannelBaseUrl = "https://lxmone.xyz"
	info.UpstreamModelName = "seedance-2.5-pro"
	info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2.5-pro": {"720p": 1}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "https://example.com/first.png", payload["image"])
	assert.Equal(t, "https://example.com/last.png", payload["image_end"])
	assert.Equal(t, false, payload["prompt_extend"])
	response := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream-id","model":"seedance-2.5-pro","status":"pending"}`))}
	upstreamID, _, taskErr := adaptor.DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-id", upstreamID)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, "seedance-2.5", payload["model"])
	assert.Equal(t, "task_public", payload["id"])
}

func TestLxmoneSettlementUsesSavedChannelRatio(t *testing.T) {
	task := &model.Task{TaskID: "public", Properties: model.Properties{OriginModelName: "seedance-2-mini", Video: &relaycommon.TaskVideoProperties{Provider: "lxmone-seedance"}},
		Data:        []byte(`{"status":"completed","video":{"duration":10}}`),
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelPrice: 0.2, GroupRatio: 1, OtherRatios: map[string]float64{"resolution": 0.5, "seconds": 5}}}}
	quota := (&TaskAdaptor{}).AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	assert.Equal(t, common.QuotaFromFloat(1*common.QuotaPerUnit), quota)
	assert.Nil(t, task.PrivateData.FinalQuotaClamp)
}

func TestLxmoneSeedanceNormalizesMediaAndPreservesSoundFlags(t *testing.T) {
	for _, fields := range []string{
		`"reference_images":["https://example.com/a.jpg",{"url":"https://example.com/b.jpg"}],"reference_videos":[{"url":"https://example.com/v.mp4"}],"reference_audios":["https://example.com/a.mp3"]`,
		`"input_reference":{"url":"https://example.com/a.jpg"},"image":"https://example.com/a.jpg","image_end":"https://example.com/b.jpg"`,
	} {
		c, info := newWanContext(t, "seedance-2", `{"prompt":"test","sound_effects":false,"no_music":true,`+fields+`}`)
		info.ChannelBaseUrl = "https://lxmone.xyz"
		info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2-pro": {"720p": 1}}
		a := &TaskAdaptor{}
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		body, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, common.DecodeJson(body, &payload))
		assert.Equal(t, false, payload["sound_effects"])
		assert.Equal(t, true, payload["no_music"])
		if strings.Contains(fields, "reference_images") {
			assert.Equal(t, []any{"https://example.com/a.jpg", "https://example.com/b.jpg"}, payload["reference_images"])
			assert.Equal(t, []any{"https://example.com/v.mp4"}, payload["reference_videos"])
			assert.Equal(t, []any{"https://example.com/a.mp3"}, payload["reference_audios"])
		} else {
			assert.Equal(t, "https://example.com/a.jpg", payload["input_reference"])
			assert.Equal(t, payload["input_reference"], payload["image"])
		}
	}
}

func TestLxmoneSeedanceRejectsInvalidMediaAndSoundBeforeBilling(t *testing.T) {
	for _, fields := range []string{
		`"sound_effects":"false"`, `"no_music":0`, `"sound_effects":null`,
		`"sound_effects":false,"no_music":false`,
		`"reference_images":[null]`, `"reference_images":null`,
		`"reference_images":[{"url":"https://example.com/a.jpg","role":"first_frame"}]`,
		`"reference_videos":["file:///tmp/a.mp4"]`,
		`"input_reference":""`, `"input_reference":null`,
		`"image":"https://user:password@example.com/a.jpg"`,
		`"input_reference":"https://example.com/a.jpg","image":"https://example.com/b.jpg"`,
	} {
		t.Run(fields, func(t *testing.T) {
			c, info := newWanContext(t, "seedance-2", `{"prompt":"test",`+fields+`}`)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
			assert.NotEqual(t, "model_price_error", err.Code)
			_, exists := c.Get("task_request")
			assert.False(t, exists)
		})
	}
}

func TestLxmoneSeedanceReferenceCounts(t *testing.T) {
	for _, tc := range []struct {
		model, field string
		count, limit int
	}{
		{"seedance-2", "reference_images", 10, 9},
		{"seedance-2-fast", "reference_videos", 4, 3},
		{"seedance-2-mini", "reference_audios", 4, 3},
		{"seedance-2.5", "reference_images", 31, 30},
		{"seedance-2.5", "reference_videos", 11, 10},
		{"seedance-2.5", "reference_audios", 11, 10},
	} {
		entries := make([]string, tc.count)
		for i := range entries {
			entries[i] = "https://example.com/media"
		}
		raw, err := common.Marshal(map[string]any{"prompt": "test", tc.field: entries})
		require.NoError(t, err)
		c, info := newWanContext(t, tc.model, string(raw))
		info.ChannelBaseUrl = "https://lxmone.xyz"
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
		require.NotNil(t, taskErr)
		assert.Equal(t, "invalid_media", taskErr.Code)
		assert.Contains(t, taskErr.Message, "at most")
	}
}
