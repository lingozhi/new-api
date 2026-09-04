package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newArgolinkSeedanceContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://argolink.io",
			ApiKey:            "provider-key",
			UpstreamModelName: constant.ArgolinkSeedance25Model,
		},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: constant.ArgolinkSeedance25Model,
	}
	return c, recorder, info
}

func TestArgolinkSeedanceValidatesAndPricesRequest(t *testing.T) {
	c, _, info := newArgolinkSeedanceContext(t, `{
		"model":"seedance-2.5",
		"prompt":"A slow aerial shot over a forest",
		"duration":4,
		"resolution":"480p",
		"aspect_ratio":"16:9"
	}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	ratios := adaptor.EstimateBilling(c, info)
	assert.Equal(t, 4.0, ratios["seconds"])
	assert.InDelta(t, 0.077/0.17, ratios["resolution"], 1e-12)
	require.NotNil(t, info.TaskRelayInfo.Video)
	assert.Equal(t, 4, info.TaskRelayInfo.Video.Duration)
	assert.Equal(t, "480p", info.TaskRelayInfo.Video.Resolution)
	assert.Equal(t, "16:9", info.TaskRelayInfo.Video.Ratio)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://argolink.io/v1/videos/generations", requestURL)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	assert.Equal(t, constant.ArgolinkSeedance25Model, payload["model"])
	assert.Equal(t, float64(4), payload["duration"])
}

func TestArgolinkSeedanceRejectsUnsafeBillingInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "duration too long", body: `{"model":"seedance-2.5","prompt":"x","duration":31}`, code: "invalid_duration"},
		{name: "multiple outputs", body: `{"model":"seedance-2.5","prompt":"x","n":2}`, code: "invalid_n"},
		{name: "unknown resolution", body: `{"model":"seedance-2.5","prompt":"x","resolution":"4K"}`, code: "invalid_resolution"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _, info := newArgolinkSeedanceContext(t, test.body)
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, test.code, taskErr.Code)
		})
	}
}

func TestArgolinkSeedanceSubmitAndPollContracts(t *testing.T) {
	c, recorder, info := newArgolinkSeedanceContext(t, `{"model":"seedance-2.5","prompt":"x"}`)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"provider-task"}`)),
	}
	upstreamID, taskData, taskErr := adaptor.DoResponse(c, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "provider-task", upstreamID)
	assert.JSONEq(t, `{"request_id":"provider-task"}`, string(taskData))
	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.JSONEq(t, `{"request_id":"task_public"}`, recorder.Body.String())

	pending, err := adaptor.ParseTaskResult([]byte(`{"model":"seedance-2.5","request_id":"provider-task","status":"pending"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, pending.Status)

	done, err := adaptor.ParseTaskResult([]byte(`{"model":"seedance-2.5","request_id":"provider-task","status":"done","video":{"duration":4,"url":"/v1/videos/provider-task/content"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, done.Status)

	failed, err := adaptor.ParseTaskResult([]byte(`{"model":"seedance-2.5","request_id":"provider-task","status":"failed","error":{"message":"provider failed","type":"api_error"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failed.Status)
	assert.Equal(t, "provider failed", failed.Reason)
}

func TestArgolinkSeedancePublicResultUsesGatewayContentURL(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: constant.ArgolinkSeedance25Model,
			Video:           &relaycommon.TaskVideoProperties{Duration: 4, Resolution: "480p"},
		},
		Data: []byte(`{"model":"seedance-2.5","request_id":"provider-task","status":"done","video":{"duration":4,"url":"/v1/videos/provider-task/content"}}`),
	}

	encoded, err := convertArgolinkSeedanceTask(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.5",
		"request_id":"task_public",
		"status":"done",
		"video":{"duration":4,"url":"/v1/videos/task_public/content"}
	}`, string(encoded))
}

func TestArgolinkReferenceVideoBillingAndDeliveredDuration(t *testing.T) {
	c, _, info := newArgolinkSeedanceContext(t, `{"model":"seedance-2.5","duration":10,"reference_videos":[{"url":"https://example.com/video.mp4"}]}`)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	ratios := adaptor.EstimateBilling(c, info)
	assert.Equal(t, 1.6, ratios["video_input"])
	task := &model.Task{Properties: model.Properties{OriginModelName: constant.ArgolinkSeedance25Model}}
	task.PrivateData.BillingContext = &model.TaskBillingContext{ModelPrice: 0.17, GroupRatio: 1, OtherRatios: ratios}
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
	for _, duration := range []int{4, 12} {
		data, err := common.Marshal(map[string]any{"video": map[string]any{"duration": duration}})
		require.NoError(t, err)
		task.Data = data
		assert.Equal(t, common.QuotaFromFloat(0.17*common.QuotaPerUnit*float64(duration)*1.6), adaptor.AdjustBillingOnComplete(task, result))
	}
	task.Data = []byte(`{"video":{"duration":-1}}`)
	assert.Zero(t, adaptor.AdjustBillingOnComplete(task, result))
	task.Data = []byte(`{"video":{"duration":9223372036854775807}}`)
	assert.Equal(t, common.QuotaFromFloat(0.17*common.QuotaPerUnit*float64(relaycommon.MaxTaskDurationSeconds)*1.6), adaptor.AdjustBillingOnComplete(task, result))
}

func TestSeedance20ModelContracts(t *testing.T) {
	for _, tc := range []struct {
		body              string
		code              string
		resolution, video float64
	}{
		{`{"model":"seedance-2.0","prompt":"forest","duration":15,"resolution":"480p"}`, "", 0.05 / 0.11, 0},
		{`{"model":"seedance-2.0","reference_videos":[{"url":"https://example.com/v.mp4"}],"resolution":"1080p"}`, "", 0.28 / 0.11, 2},
		{`{"model":"seedance-2.0","prompt":"forest","duration":16}`, "invalid_duration", 0, 0},
		{`{"model":"seedance-2.0","prompt":"forest","resolution":"1080p"}`, "invalid_resolution", 0, 0},
	} {
		c, _, info := newArgolinkSeedanceContext(t, tc.body)
		info.OriginModelName, info.UpstreamModelName = constant.ArgolinkSeedance20Model, constant.ArgolinkSeedance20Model
		a := &TaskAdaptor{}
		a.Init(info)
		err := a.ValidateRequestAndSetAction(c, info)
		if tc.code != "" {
			require.NotNil(t, err)
			assert.Equal(t, tc.code, err.Code)
			continue
		}
		require.Nil(t, err)
		ratios := a.EstimateBilling(c, info)
		assert.InDelta(t, tc.resolution, ratios["resolution"], 1e-12)
		assert.Equal(t, tc.video, ratios["video_input"])
		body, buildErr := a.BuildRequestBody(c, info)
		require.NoError(t, buildErr)
		data, readErr := io.ReadAll(body)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), `"model":"seedance-2.0"`)
		task := &model.Task{TaskID: "task_20", Properties: model.Properties{OriginModelName: constant.ArgolinkSeedance20Model}}
		public, convertErr := a.ConvertToOpenAIVideo(task)
		require.NoError(t, convertErr)
		assert.Contains(t, string(public), `"model":"seedance-2.0"`)
	}
}
