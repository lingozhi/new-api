package image_stream

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurableWorkerDeliversWanAndSeedanceWebhooks(t *testing.T) {
	setupAsyncImageSubmitTestDB(t)
	previousFactory, previousSender := service.GetTaskAdaptorFunc, sendAsyncImageWebhook
	service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &sora.TaskAdaptor{} }
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousFactory; sendAsyncImageWebhook = previousSender })
	type delivery struct {
		id, secret string
		payload    any
	}
	deliveries := make(chan delivery, 2)
	sendAsyncImageWebhook = func(_ context.Context, _ string, secret, id string, payload any) error {
		deliveries <- delivery{id, secret, payload}
		return nil
	}
	for _, tc := range []struct {
		model, provider string
		status          model.TaskStatus
	}{
		{"wan3.0", "wan-unified", model.TaskStatusSuccess},
		{"seedance-2", "lxmone-seedance", model.TaskStatusFailure},
	} {
		task := &model.Task{TaskID: "task_" + tc.model, Platform: "3", Status: tc.status, FailReason: "generation failed", Properties: model.Properties{OriginModelName: tc.model, Video: &relaycommon.TaskVideoProperties{Provider: tc.provider}}}
		hook := &model.TaskWebhook{URL: "https://8.8.8.8/hook", Secret: "test-secret"}
		require.NoError(t, model.InsertTaskWithWebhook(task, hook))
	}
	assert.True(t, (asyncImageSystemTaskHandler{}).Enabled())
	delivered, retried, err := deliverDueImageWebhooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, delivered)
	assert.Zero(t, retried)
	close(deliveries)
	for event := range deliveries {
		assert.Equal(t, "test-secret", event.secret)
		payload, ok := event.payload.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, event.id, payload["id"])
		raw, err := common.Marshal(payload)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "test-secret")
		if event.id == "task_wan3.0" {
			assert.Equal(t, "completed", payload["status"])
		} else {
			assert.Equal(t, "failed", payload["status"])
		}
		var hook model.TaskWebhook
		require.NoError(t, model.DB.Where("task_id = ?", event.id).First(&hook).Error)
		assert.Equal(t, model.TaskWebhookStatusDelivered, hook.Status)
	}
}
