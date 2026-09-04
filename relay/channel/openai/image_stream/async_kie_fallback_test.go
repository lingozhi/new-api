package image_stream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageRejectedGenerationUsesKIEOnce(t *testing.T) {
	failed, backup, task := setupCheckpointedProviderFailoverTest(t, "kie-rejected-generation")
	routing := asyncImageFailoverTestRouting()
	routing.Profiles[0].Protocol = dto.ImageRoutingProtocolKIEJobs
	routing.Profiles[0].UpstreamPath = "/api/v1/jobs/createTask"
	backup.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: routing})
	require.NoError(t, model.DB.Save(backup).Error)

	other := *failed
	other.Id = 9120
	priority := int64(100)
	other.Priority = &priority
	require.NoError(t, model.DB.Create(&other).Error)
	model.SetChannelCacheForTest(map[int]*model.Channel{failed.Id: failed, backup.Id: backup, other.Id: &other}, map[string]map[string][]int{
		"default": {"gpt-image-2": {other.Id, failed.Id, backup.Id}},
	})

	task.Quota = 100
	task.PrivateData.BillingSource = "wallet"
	require.NoError(t, model.DB.Save(task).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", 900).Error)
	require.NoError(t, model.DB.Create(&model.ImageBillingReservation{
		TaskID: task.TaskID, UserID: task.UserId, ExpectedQuota: 100,
		FundingSource: "wallet", WalletReserved: 100, Status: model.ImageBillingReservationActive,
	}).Error)
	var calls []int
	installAsyncImageFailoverExecutor(t, func(request *GenericImageExecutionRequest) (*GenericImageExecutionResult, *types.NewAPIError) {
		calls = append(calls, request.RelayInfo.ChannelId)
		assert.Equal(t, "draw a reliable image", request.ImageRequest.Prompt)
		require.NoError(t, request.BeforeProviderCall())
		if request.RelayInfo.ChannelId == failed.Id {
			apiErr := types.NewErrorWithStatusCode(fmt.Errorf("%w: The generated images appear to be unsafe", ErrGenericImageDefinitiveResponse), types.ErrorCodeBadResponseStatusCode, http.StatusUnavailableForLegalReasons)
			apiErr.UpstreamStatusCode = http.StatusUnavailableForLegalReasons
			return nil, apiErr
		}
		require.NoError(t, request.Checkpoint(&GenericImageUpstreamResponse{StatusCode: http.StatusOK, Body: []byte(`{"code":200,"data":{"taskId":"kie-test"}}`)}))
		return nil, types.NewErrorWithStatusCode(errors.New("image_unsafe"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	})
	completed, err := executeAsyncImageTask(context.Background(), task)
	assertAsyncImageFailoverScheduled(t, failed, backup, task, completed, err)
	assert.False(t, model.IsChannelCoolingDown(failed.Id))
	assert.True(t, decodeStoredAsyncImagePayload(t, task.CheckpointData).KIEFallback)
	var user model.User
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	assert.Equal(t, 900, user.Quota)
	claimed, err := model.ClaimImageTask(task, common.GetTimestamp())
	require.NoError(t, err)
	require.True(t, claimed)
	completed, err = executeAsyncImageTask(context.Background(), task)
	require.NoError(t, err)
	assert.False(t, completed)
	assert.Equal(t, []int{failed.Id, backup.Id}, calls)
	require.NoError(t, model.DB.First(&user, task.UserId).Error)
	assert.Equal(t, 1000, user.Quota)
	finalization, err := model.FinalizeImageTask(task.TaskID)
	require.NoError(t, err)
	assert.False(t, finalization.Applied)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), finalization.Task.Status)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Contains(t, stored.FailReason, "image_unsafe")
}

func TestAsyncImageContentRejectionDoesNotMatchGenericErrors(t *testing.T) {
	for _, message := range []string{"invalid image size", "unavailable for legal reasons", "request timed out"} {
		t.Run(message, func(t *testing.T) {
			assert.False(t, isAsyncImageContentRejection(types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponseStatusCode, http.StatusUnavailableForLegalReasons)))
		})
	}
}
