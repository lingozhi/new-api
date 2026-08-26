package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasksToDtoHidesWorkflowProviderFromUsers(t *testing.T) {
	task := &model.Task{
		ID:         1,
		TaskID:     "task_public_video",
		Platform:   constant.TaskPlatformAutoDL,
		ChannelId:  60,
		Action:     constant.TaskActionVideoGenerationV2,
		Status:     model.TaskStatusSuccess,
		FailReason: "AutoDL internal detail",
		Data:       []byte(`{"data":{"results":[{"url":"https://provider.example/result.mp4"}]}}`),
		Properties: model.Properties{OriginModelName: constant.AutoDLModelMiniMaxH3},
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://provider.example/result.mp4",
		},
	}

	items := tasksToDto([]*model.Task{task}, false)
	require.Len(t, items, 1)
	assert.Equal(t, "minimax", items[0].Platform)
	assert.Zero(t, items[0].ChannelId)
	assert.Empty(t, items[0].FailReason)
	assert.Equal(t, taskcommon.BuildProxyURL(task.TaskID), items[0].ResultURL)
	assert.Empty(t, items[0].Data)
	assert.NotContains(t, items[0].ResultURL, "provider.example")
}

func TestTasksToDtoHidesArbitraryWorkflowFailureDetailsFromUsers(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public_failed_video",
		Platform:   constant.TaskPlatformAutoDL,
		ChannelId:  60,
		Action:     constant.TaskActionVideoGenerationV2,
		Status:     model.TaskStatusFailure,
		FailReason: "provider.example rejected the prompt",
		Properties: model.Properties{OriginModelName: constant.AutoDLModelMiniMaxH3},
	}

	items := tasksToDto([]*model.Task{task}, false)
	require.Len(t, items, 1)
	assert.Equal(t, "Video generation failed", items[0].FailReason)
	assert.NotContains(t, items[0].FailReason, "provider.example")
}

func TestTasksToDtoKeepsProviderDiagnosticsForAdmins(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_admin_video",
		Platform:   constant.TaskPlatformAutoDL,
		ChannelId:  60,
		Status:     model.TaskStatusSuccess,
		FailReason: "AutoDL internal detail",
		Data:       []byte(`{"data":{"status":"SUCCESS"}}`),
		Properties: model.Properties{OriginModelName: constant.AutoDLModelMiniMaxH3},
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://provider.example/result.mp4",
		},
	}

	items := tasksToDto([]*model.Task{task}, true)
	require.Len(t, items, 1)
	assert.Equal(t, string(constant.TaskPlatformAutoDL), items[0].Platform)
	assert.Equal(t, 60, items[0].ChannelId)
	assert.Equal(t, "AutoDL internal detail", items[0].FailReason)
	assert.Equal(t, "https://provider.example/result.mp4", items[0].ResultURL)
	assert.NotEmpty(t, items[0].Data)
}
