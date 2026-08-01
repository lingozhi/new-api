package image_stream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageTerminalProviderFailureSwitchesCheckpointedTaskToBackup(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "terminal-provider-failure")

	genericImageExecutorRegistry.Lock()
	previousExecutor := genericImageExecutorRegistry.executor
	genericImageExecutorRegistry.executor = func(_ context.Context, request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		require.NoError(t, request.BeforeProviderCall())
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{
			StatusCode: http.StatusAccepted,
			Body:       json.RawMessage(`{"task_id":"upstream-task-failed"}`),
		}))
		return nil, types.NewErrorWithStatusCode(
			errors.New("provider task reached terminal failed state"),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
	}
	genericImageExecutorRegistry.Unlock()
	t.Cleanup(func() {
		genericImageExecutorRegistry.Lock()
		genericImageExecutorRegistry.executor = previousExecutor
		genericImageExecutorRegistry.Unlock()
	})

	completed, executeErr := executeAsyncImageTask(context.Background(), task)

	assert.False(t, completed)
	assert.ErrorIs(t, executeErr, errAsyncImageRetryScheduled,
		"a definitively failed checkpointed provider task should atomically requeue on a backup channel")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusNotStart), stored.Status)
	assert.Equal(t, backup.Id, stored.ChannelId)
	assert.Equal(t, 1, stored.ProviderAttempts)
	assert.NotEqual(t, failed.Id, stored.ChannelId)
	if assert.NotEmpty(t, stored.CheckpointData, "the switched task must retain its next-provider checkpoint") {
		payload := decodeStoredAsyncImagePayload(t, stored.CheckpointData)
		assert.False(t, payload.ProviderStored)
		assert.False(t, payload.ProviderCallStarted)
		assert.Equal(t, []int{failed.Id, backup.Id}, payload.AttemptedChannelIDs)
	}
}

func setupCheckpointedProviderFailoverTest(t *testing.T, suffix string) (*model.Channel, *model.Channel, *model.Task) {
	t.Helper()
	setupAsyncImageSubmitTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ImageTaskArtifactChunk{}, &model.Log{}))

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	model.ClearChannelCacheForTest()
	model.ClearChannelCooldownsForTest()
	t.Cleanup(func() {
		model.ClearChannelCacheForTest()
		model.ClearChannelCooldownsForTest()
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})
	previousLogDB := model.LOG_DB
	model.LOG_DB = model.DB
	t.Cleanup(func() { model.LOG_DB = previousLogDB })

	priority := int64(10)
	weight := uint(100)
	failedBaseURL := "https://failed-" + suffix + ".example.com"
	backupBaseURL := "https://backup-" + suffix + ".example.com"
	failed := &model.Channel{
		Id: 9116, Type: constant.ChannelTypeOpenAI, Key: "failed-key", Status: common.ChannelStatusEnabled,
		Name: "failed " + suffix, CreatedTime: 1700001100, BaseURL: &failedBaseURL,
		Models: "gpt-image-2", Group: "default", Priority: &priority, Weight: &weight,
	}
	backup := &model.Channel{
		Id: 9118, Type: constant.ChannelTypeOpenAI, Key: "backup-key", Status: common.ChannelStatusEnabled,
		Name: "backup " + suffix, CreatedTime: 1700001200, BaseURL: &backupBaseURL,
		Models: "gpt-image-2", Group: "default", Priority: &priority, Weight: &weight,
	}
	failed.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	backup.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: asyncImageFailoverTestRouting()})
	require.NoError(t, model.DB.Create(failed).Error)
	require.NoError(t, model.DB.Create(backup).Error)
	model.SetChannelCacheForTest(map[int]*model.Channel{failed.Id: failed, backup.Id: backup}, map[string]map[string][]int{
		"default": {"gpt-image-2": {failed.Id, backup.Id}},
	})

	user := &model.User{
		Username: "async-failover-" + suffix,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, model.DB.Create(user).Error)
	payload := asyncImageTaskPayload{
		Version:                  asyncImagePayloadVersion,
		Executor:                 AsyncImageExecutorAdaptor,
		RelayMode:                relayconstant.RelayModeImagesGenerations,
		ImageRoutingProtocol:     dto.ImageRoutingProtocolImagesGenerations,
		ImageRoutingUpstreamPath: "/v1/images/generations",
		ImageRequirement: &dto.ImageSelectionRequirement{
			Operation: dto.ImageOperationGeneration,
			Size:      "auto",
			N:         1,
		},
		Request:             &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw a reliable image", Size: "auto", ResponseFormat: "url"},
		ChannelType:         failed.Type,
		ChannelCreateTime:   failed.CreatedTime,
		AttemptedChannelIDs: []int{failed.Id},
		PreparedRequest: &PreparedAsyncImageRequest{
			Body:                     []byte(`{"model":"gpt-image-2","prompt":"draw a reliable image","size":"auto","response_format":"url"}`),
			RelayMode:                relayconstant.RelayModeImagesGenerations,
			ContentType:              "application/json",
			RequestURLPath:           "/v1/images/generations",
			ImageRoutingProtocol:     dto.ImageRoutingProtocolImagesGenerations,
			ImageRoutingUpstreamPath: "/v1/images/generations",
			APIType:                  constant.APITypeOpenAI,
			ChannelType:              failed.Type,
			ChannelCreateTime:        failed.CreatedTime,
		},
	}
	task := &model.Task{
		TaskID: "task_" + suffix, Platform: constant.TaskPlatformOpenAIImage,
		UserId: user.Id, ChannelId: failed.Id, Status: model.TaskStatusInProgress,
		Attempt: 1, Progress: "10%",
		Properties: model.Properties{OriginModelName: "gpt-image-2", UpstreamModelName: "gpt-image-2"},
		PrivateData: model.TaskPrivateData{
			ChannelKeyHash: common.GenerateHMAC(failed.Key),
			BillingContext: &model.TaskBillingContext{PerCallBilling: true, OriginModelName: "gpt-image-2"},
		},
	}
	task.SetCheckpointData(payload)
	require.NoError(t, model.DB.Create(task).Error)
	return failed, backup, task
}

func asyncImageFailoverTestRouting() *dto.ImageRoutingConfig {
	return &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{{
			Model:               "gpt-image-2",
			Protocol:            dto.ImageRoutingProtocolImagesGenerations,
			UpstreamPath:        "/v1/images/generations",
			Operations:          []dto.ImageOperation{dto.ImageOperationGeneration},
			Sizes:               []string{"auto"},
			DefaultSize:         "auto",
			MaxOutputImages:     1,
			AllowedCombinations: []dto.ImageRoutingCombination{{Operation: dto.ImageOperationGeneration, Size: "auto"}},
			VerificationStatus:  dto.ImageRoutingVerificationProductionVerified,
		}},
	}
}
