package sora

import (
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedance25FixedSpecSelectsSD25BeforeCheckpoint(t *testing.T) {
	for _, tc := range []struct{ body, target string }{
		{`"seconds":"30","size":"720P"`, "sd-2.5"},
		{`"duration":30`, "sd-2.5"},
		{`"seconds":30,"duration":30,"resolution":"720p","size":"720P"`, "sd-2.5"},
		{`"duration":29,"resolution":"720p"`, "seedance-2.5-pro"},
		{`"duration":30,"resolution":"480p"`, "seedance-2.5-pro"},
		{`"duration":30,"resolution":"1080p"`, "seedance-2.5-pro"},
		{`"duration":30,"resolution":"4k"`, "seedance-2.5-pro"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c, info := newWanContext(t, "seedance-2.5", `{"model":"seedance-2.5","prompt":"test","reference_images":[{"url":"https://example.com/ref.png"}],"webhook_url":"https://8.8.8.8/hook","webhook_secret":"test-secret",`+tc.body+`}`)
			info.ChannelBaseUrl = "https://lxmone.xyz"
			info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2.5-pro": {"480p": 1, "720p": 1, "1080p": 1}}
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.DecodeJson(body, &payload))
			assert.Equal(t, tc.target, payload["model"])
			assert.Equal(t, "seedance-2.5", info.OriginModelName)
			assert.NotContains(t, payload, "webhook_secret")
			assert.NotContains(t, payload, "webhook_url")
			if tc.target == "sd-2.5" {
				assert.Equal(t, "sd-2.5", info.UpstreamModelName)
				assert.True(t, info.IsModelMapped)
				assert.Equal(t, map[string]any{"model": "sd-2.5", "prompt": "test", "resolution": "720p", "aspect_ratio": "16:9", "images": []any{"https://example.com/ref.png"}}, payload)
				assert.Equal(t, float64(30), a.EstimateBilling(c, info)["seconds"])
			}
		})
	}
}

func TestSD25RejectsUnsupportedInputsWithoutFallingBack(t *testing.T) {
	for _, extra := range []string{
		`"reference_videos":["https://example.com/ref.mp4"]`,
		`"reference_audios":["https://example.com/ref.mp3"]`,
		`"image":"https://example.com/first.png"`,
		`"sound_effects":false`,
		`"prompt_extend":false`,
		`"reference_images":["https://example.com/1","https://example.com/2","https://example.com/3","https://example.com/4","https://example.com/5","https://example.com/6","https://example.com/7","https://example.com/8","https://example.com/9","https://example.com/10","https://example.com/11"]`,
	} {
		c, info := newWanContext(t, "seedance-2.5", `{"prompt":"test","duration":30,"resolution":"720p",`+extra+`}`)
		info.ChannelBaseUrl = "https://lxmone.xyz"
		err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
		require.NotNil(t, err, extra)
		assert.Equal(t, http.StatusBadRequest, err.StatusCode)
		assert.Equal(t, "unsupported_sd25_input", err.Code)
	}
}

func TestSD25FixedSpecKeepsReservedCharge(t *testing.T) {
	task := &model.Task{Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "seedance-2.5", UpstreamModelName: "sd-2.5", Video: &relaycommon.TaskVideoProperties{Provider: "lxmone-seedance", Duration: 30, Resolution: "720p"}}, Data: []byte(`{"video":{"duration":99}}`)}
	task.PrivateData.BillingContext = &model.TaskBillingContext{ModelPrice: 1, GroupRatio: 1, OtherRatios: map[string]float64{"seconds": 30, "resolution": 1}}
	assert.Zero(t, (&TaskAdaptor{}).AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}), "fixed specification retains the original charge")
}

func TestSD25SubmissionAndPollingKeepPublicModel(t *testing.T) {
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v1/videos" {
			var body map[string]any
			if err := common.DecodeJson(r.Body, &body); err != nil {
				http.Error(w, "bad JSON", 400)
				return
			}
			captured <- body
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"provider_task","model":"sd-2.5","status":"queued"}`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/videos/provider_task" {
			_, _ = io.WriteString(w, `{"id":"provider_task","model":"sd-2.5","status":"completed","video":{"duration":30}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	original, info := newWanContext(t, "seedance-2.5", `{"model":"seedance-2.5","prompt":"test","seconds":30,"size":"720P","webhook_url":"https://8.8.8.8/hook","webhook_secret":"private"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = original.Request
	info.ChannelBaseUrl = "https://lxmone.xyz"
	info.PublicTaskID = "task_public"
	info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2.5-pro": {"720p": 1}}
	a := &TaskAdaptor{}
	a.Init(info)
	// Only the transport destination is local; provider selection retains Lxmone context.
	a.baseURL = server.URL
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	task := model.InitTask("3", info)
	assert.Equal(t, "sd-2.5", task.Properties.UpstreamModelName)
	response, err := a.DoRequest(c, info, body)
	require.NoError(t, err)
	id, _, taskErr := a.DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "provider_task", id)
	payload := <-captured
	assert.Equal(t, "sd-2.5", payload["model"])
	assert.NotContains(t, payload, "webhook_secret")
	var public map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &public))
	assert.Equal(t, "seedance-2.5", public["model"])
	assert.Equal(t, "task_public", public["id"])
	response, err = a.FetchTask(server.URL, "test", map[string]any{"task_id": id}, "")
	require.NoError(t, err)
	defer response.Body.Close()
	task.Data, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	task.Status = model.TaskStatusSuccess
	result, err := a.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(result, &public))
	assert.Equal(t, "seedance-2.5", public["model"])
	assert.Equal(t, "completed", public["status"])
	hook, handled, err := a.BuildTaskWebhookPayload(task)
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, "seedance-2.5", hook.(map[string]any)["model"])
	assert.Equal(t, "/v1/videos/task_public/content", hook.(map[string]any)["content_url"])
}
