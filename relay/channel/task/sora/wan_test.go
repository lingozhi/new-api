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
	info := &relaycommon.RelayInfo{OriginModelName: modelName, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelName, ChannelBaseUrl: "https://provider.example"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	return c, info
}

func TestWanOfficialParametersMatchUpstreamAndBilling(t *testing.T) {
	for _, name := range []string{"wan3.0", "wan3.0-video"} {
		for _, tc := range []struct {
			parameters        string
			duration, reserve int
			resolution, ratio string
			cost              float64
		}{
			{`{}`, 5, 5, "1080P", "adaptive", 1.75},
			{`{"duration":2,"resolution":"480P","ratio":"4:3","audio":false,"prompt_extend":false,"watermark":false,"seed":0}`, 2, 2, "480P", "4:3", .5},
			{`{"duration":30,"resolution":"720P","ratio":"3:4","seed":2147483647}`, 30, 30, "720P", "3:4", 9},
			{`{"duration":-1,"resolution":"480P"}`, -1, 30, "480P", "adaptive", 7.5},
		} {
			t.Run(name+tc.parameters, func(t *testing.T) {
				c, info := newWanContext(t, name, `{"model":"`+name+`","input":{"prompt":"test"},"parameters":`+tc.parameters+`}`)
				a := &TaskAdaptor{}
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				ratios := a.EstimateBilling(c, info)
				assert.InDelta(t, tc.cost, .3*ratios["seconds"]*ratios["resolution"], 1e-9)
				assert.Equal(t, float64(tc.reserve), ratios["seconds"])
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.DecodeJson(reader, &payload))
				assert.Equal(t, float64(tc.duration), payload["duration"])
				assert.Equal(t, tc.duration, info.TaskRelayInfo.Video.Duration)
				assert.Equal(t, tc.resolution, payload["resolution"])
				assert.Equal(t, tc.ratio, payload["aspect_ratio"])
				assert.Equal(t, "wan3.0-video", payload["model"])
				for _, removed := range []string{"input", "parameters", "mode", "speed", "size", "ratio", "n", "webhook_url", "webhook_secret"} {
					assert.NotContains(t, payload, removed)
				}
				if tc.duration == 2 {
					assert.Equal(t, false, payload["audio"])
					assert.Equal(t, false, payload["prompt_extend"])
					assert.Equal(t, false, payload["watermark"])
					assert.Equal(t, float64(0), payload["seed"])
				}
				if tc.parameters == `{}` {
					assert.Equal(t, true, payload["audio"])
					assert.Equal(t, true, payload["prompt_extend"])
					assert.Equal(t, false, payload["watermark"])
					assert.Equal(t, float64(-1), payload["seed"])
				}
			})
		}
	}
}

func TestWanOfficialMediaCombinationsAndOrder(t *testing.T) {
	for _, types := range [][]string{{"first_frame"}, {"first_frame", "last_frame"}, {"reference_video", "reference_image", "reference_audio", "reference_image"}, {"file", "reference_audio"}, {"link", "reference_image"}} {
		t.Run(strings.Join(types, "+"), func(t *testing.T) {
			media := make([]wanMedia, len(types))
			for i, kind := range types {
				media[i] = wanMedia{Type: kind, URL: "https://example.com/asset"}
			}
			raw, err := common.Marshal(map[string]any{"model": "wan3.0", "input": map[string]any{"media": media}, "parameters": map[string]any{"duration": 2, "resolution": "480P"}})
			require.NoError(t, err)
			c, info := newWanContext(t, "wan3.0", string(raw))
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			var payload struct {
				Media []wanMedia `json:"media"`
			}
			require.NoError(t, common.DecodeJson(reader, &payload))
			assert.Equal(t, media, payload.Media)
		})
	}
}

func TestWanRejectsInvalidOfficialInputsBeforeBilling(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"input":{}}`, `{"input":null}`, `{"input":{"prompt":null}}`, `{"input":{"prompt":1}}`, `{"input":{"media":[null]}}`,
		`{"input":{"prompt":"test","media":null}}`, `{"input":{"prompt":"test","extra":1}}`,
		`{"input":{"prompt":"test"},"parameters":null}`, `{"input":{"prompt":"test"},"parameters":{"duration":0}}`,
		`{"input":{"prompt":"test"},"parameters":{"duration":-2}}`, `{"input":{"prompt":"test"},"parameters":{"duration":31}}`,
		`{"input":{"prompt":"test"},"parameters":{"duration":18446744073686646784}}`, `{"input":{"prompt":"test"},"parameters":{"duration":2.5}}`,
		`{"input":{"prompt":"test"},"parameters":{"duration":"2"}}`, `{"input":{"prompt":"test"},"parameters":{"resolution":"4K"}}`,
		`{"input":{"prompt":"test"},"parameters":{"resolution":"480p"}}`, `{"input":{"prompt":"test"},"parameters":{"ratio":"2:1"}}`,
		`{"input":{"prompt":"test"},"parameters":{"audio":"false"}}`, `{"input":{"prompt":"test"},"parameters":{"seed":-2}}`,
		`{"input":{"prompt":"test"},"parameters":{"seed":2147483648}}`, `{"input":{"prompt":"test"},"parameters":{"seed":18446744073686646784}}`,
		`{"input":{"prompt":"test"},"parameters":{"watermark":null}}`, `{"input":{"prompt":"test"},"parameters":{"seconds":2}}`,
		`{"input":{"media":[{"type":"last_frame","url":"https://example.com/a"}]}}`,
		`{"input":{"media":[{"type":"first_frame","url":"https://example.com/a"},{"type":"reference_audio","url":"https://example.com/b"}]}}`,
		`{"input":{"media":[{"type":"file","url":"https://example.com/a"},{"type":"link","url":"https://example.com/b"}]}}`,
		`{"input":{"media":[{"type":"reference_video","url":"https://example.com/a","duration":1e100}]}}`,
		`{"input":{"media":[{"type":"reference_video","url":"https://example.com/a"}]},"parameters":{"duration":30}}`,
		`{"input":{"media":[{"type":"audio","url":"https://example.com/a"}]}}`,
		`{"input":{"media":[{"type":"reference_image","url":"https://user:password@example.com/a"}]}}`,
		`{"input":{"media":[{"type":"reference_image","url":"file:///tmp/a"}]}}`,
		`{"input":{"media":[{"type":"reference_image","url":"oss://dashscope-instant/a"}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			body = `{"model":"wan3.0",` + body[1:]
			c, info := newWanContext(t, "wan3.0", body)
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, 400, err.StatusCode)
			_, exists := c.Get("task_request")
			assert.False(t, exists)
		})
	}
}

func TestWanRejectsRemovedFlatParametersForBothNames(t *testing.T) {
	for _, name := range []string{"wan3.0", "wan3.0-video", "wan3.0-video-prime"} {
		for _, field := range []string{`"prompt":"test"`, `"mode":"auto"`, `"speed":"standard"`, `"seconds":"2"`, `"duration":2`, `"size":"480P"`, `"resolution":"480P"`, `"aspect_ratio":"16:9"`, `"ratio":"adaptive"`, `"n":1`, `"prompt_extend":false`, `"audio":false`, `"seed":0`, `"watermark":false`, `"first_frame":"https://example.com/a"`, `"last_frame":"https://example.com/a"`, `"reference_images":[]`, `"reference_videos":[]`, `"reference_audios":[]`, `"media":[]`} {
			t.Run(name+field, func(t *testing.T) {
				c, info := newWanContext(t, name, `{"model":"`+name+`","input":{"prompt":"test"},`+field+`}`)
				err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
				require.NotNil(t, err)
				assert.Equal(t, 400, err.StatusCode)
				_, exists := c.Get("task_request")
				assert.False(t, exists)
			})
		}
	}
}

func TestWanMediaLimitsAndDataURLs(t *testing.T) {
	for kind, limit := range map[string]int{"first_frame": 1, "reference_image": 10, "reference_video": 5, "reference_audio": 5, "file": 1, "link": 1} {
		media := make([]wanMedia, limit+1)
		for i := range media {
			media[i] = wanMedia{Type: kind, URL: "https://example.com/asset"}
		}
		raw, err := common.Marshal(map[string]any{"model": "wan3.0", "input": map[string]any{"media": media}})
		require.NoError(t, err)
		c, info := newWanContext(t, "wan3.0", string(raw))
		require.NotNil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	}
	for _, tc := range []struct {
		kind, url string
		valid     bool
	}{
		{"reference_image", "http://example.com/a.png", true}, {"reference_image", "data:image/png;base64,YWJj", true},
		{"reference_image", "data:image/png;base64,???", false}, {"reference_image", "data:image/png;base64,", false},
		{"reference_image", "data:text/html;base64,YWJj", false}, {"reference_audio", "data:image/png;base64,YWJj", false},
	} {
		err := validateWanMediaURL(wanMedia{Type: tc.kind, URL: tc.url})
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestWanPromptTruncatesUnicodeCharacters(t *testing.T) {
	raw, err := common.Marshal(map[string]any{"model": "wan3.0", "input": map[string]any{"prompt": strings.Repeat("万", 20001)}})
	require.NoError(t, err)
	c, info := newWanContext(t, "wan3.0", string(raw))
	a := &TaskAdaptor{}
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, common.DecodeJson(reader, &body))
	assert.Equal(t, strings.Repeat("万", 20000), body["prompt"])
}

func TestWanAutomaticSettlementReleasesReserveAndBoundsMetadata(t *testing.T) {
	for _, tc := range []struct {
		data    string
		seconds float64
	}{
		{`{"video":{"duration":4}}`, 4}, {`{"completed_at":"2026-09-12T05:18:41Z","video":{"duration":2.5}}`, 2.5},
		{`{"video":{"duration":1e100}}`, 30}, {`{"video":{"duration":0}}`, 2}, {`{"video":{"duration":-1}}`, 2}, {`{}`, 2},
	} {
		t.Run(tc.data, func(t *testing.T) {
			task := &model.Task{TaskID: "public", Data: []byte(tc.data), Properties: model.Properties{OriginModelName: "wan3.0", Video: &relaycommon.TaskVideoProperties{Provider: "wan-unified", Duration: -1}}, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelPrice: .3, GroupRatio: 1.6, OtherRatios: map[string]float64{"seconds": 30, "resolution": .25 / .3}}}}
			a := &TaskAdaptor{}
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
			assert.Equal(t, common.QuotaFromFloat(.25*common.QuotaPerUnit*tc.seconds*1.6), a.AdjustBillingOnComplete(task, result))
			assert.Zero(t, a.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure}))
			task.Properties.Video.Duration = 2
			assert.Zero(t, a.AdjustBillingOnComplete(task, result))
		})
	}
}

func TestWanSubmissionURLAndRetiredModels(t *testing.T) {
	c, info := newWanContext(t, "wan3.0", `{"model":"wan3.0","input":{"prompt":"test"}}`)
	info.ChannelBaseUrl = "https://tokens.aijiakefu.com/v1/"
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	endpoint, err := a.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://tokens.aijiakefu.com/v1/videos/generations", endpoint)
	for _, name := range []string{"wan3.0-prime-r2v", "wan3.0-i2v"} {
		c, info := newWanContext(t, name, `{"model":"`+name+`","input":{"prompt":"test"}}`)
		require.NotNil(t, a.ValidateRequestAndSetAction(c, info))
	}
	for _, path := range []string{"/v1/videos/task_x/remix", "/v1/videos/generations"} {
		c, info := newWanContext(t, "wan3.0", `{"model":"wan3.0","input":{"prompt":"test"}}`)
		c.Request.URL.Path = path
		require.NotNil(t, a.ValidateRequestAndSetAction(c, info))
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
	for _, name := range []string{"wan3.0", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{OriginModelName: name, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "wan3.0-video"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
			response := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream","model":"wan3.0-video","status":"queued"}`))}
			upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, response, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "upstream", upstreamID)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			for _, key := range []string{"id", "task_id", "request_id"} {
				assert.Equal(t, "task_public", payload[key])
			}
			assert.Equal(t, name, payload["model"])
		})
	}
}

func TestAijiauWanPollingStatusAliases(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   string
	}{
		{"pending", model.TaskStatusQueued},
		{"running", model.TaskStatusInProgress},
		{"archiving", model.TaskStatusInProgress},
		{"succeeded", model.TaskStatusSuccess},
		{"success", model.TaskStatusSuccess},
		{"completed", model.TaskStatusSuccess},
		{"canceled", model.TaskStatusFailure},
		{"rejected", model.TaskStatusFailure},
	} {
		t.Run(tc.status, func(t *testing.T) {
			body, err := common.Marshal(map[string]string{"id": "provider-id", "model": "wan3.0-video", "status": tc.status})
			require.NoError(t, err)
			result, err := (&TaskAdaptor{}).ParseTaskResult(body)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Status)
		})
	}
}

func TestAijiauWanCompletionAcceptsProviderMetadata(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"completed_at":"2026-09-12T05:18:41.229Z",
		"id":"provider-id",
		"model":"wan3.0-video",
		"status":"completed",
		"video":{"duration":2,"resolution":"480P","url":"https://example.com/result.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
}

func TestWanPrimeKeepsModelIdentityAndResolutionPricing(t *testing.T) {
	for _, tc := range []struct {
		resolution string
		cost       float64
	}{
		{"480P", .90}, {"720P", 1.80}, {"1080P", 3.60},
	} {
		t.Run(tc.resolution, func(t *testing.T) {
			c, info := newWanContext(t, "wan3.0-video-prime", `{"model":"wan3.0-video-prime","input":{"prompt":"test"},"parameters":{"duration":2,"resolution":"`+tc.resolution+`","audio":false,"seed":0}}`)
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			// A configured mapping must not silently turn a Prime request into standard.
			info.UpstreamModelName = "wan3.0-video"
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, common.DecodeJson(reader, &body))
			assert.Equal(t, "wan3.0-video-prime", body["model"])
			assert.Equal(t, "wan3.0-video-prime", info.UpstreamModelName)
			assert.Equal(t, false, body["audio"])
			assert.Equal(t, float64(0), body["seed"])
			ratios := a.EstimateBilling(c, info)
			assert.InDelta(t, tc.cost, .90*ratios["seconds"]*ratios["resolution"], 1e-9)
		})
	}
}

func TestWanPrimeAutomaticSettlementUsesPrimePriceSnapshot(t *testing.T) {
	task := &model.Task{Data: []byte(`{"video":{"duration":2}}`), Properties: model.Properties{OriginModelName: "wan3.0-video-prime", Video: &relaycommon.TaskVideoProperties{Provider: "wan-unified", Duration: -1}}, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelPrice: .90, GroupRatio: 1, OtherRatios: map[string]float64{"seconds": 30, "resolution": .5}}}}
	a := &TaskAdaptor{}
	assert.Equal(t, common.QuotaFromFloat(.90*common.QuotaPerUnit), a.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}))
	assert.Zero(t, a.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure}))
}
