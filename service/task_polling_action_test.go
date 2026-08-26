package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type actionAwarePollingAdaptor struct {
	responseBody    []byte
	fetchAction     string
	parseActions    []string
	sanitizeActions []string
	legacyParses    int
	taskResult      *relaycommon.TaskInfo
	parseErr        error
	validationErr   error
	refundFailure   *bool
}

func (a *actionAwarePollingAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *actionAwarePollingAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	a.fetchAction, _ = body["action"].(string)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(a.responseBody)),
	}, nil
}

func (a *actionAwarePollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	a.legacyParses++
	return nil, nil
}

func (a *actionAwarePollingAdaptor) ParseTaskResultForAction(_ []byte, action string) (*relaycommon.TaskInfo, error) {
	a.parseActions = append(a.parseActions, action)
	if a.parseErr != nil {
		return nil, a.parseErr
	}
	if a.taskResult != nil {
		result := *a.taskResult
		return &result, nil
	}
	return &relaycommon.TaskInfo{
		TaskID:   "provider-index-tts-task",
		Status:   model.TaskStatusInProgress,
		Progress: "30%",
	}, nil
}

func (a *actionAwarePollingAdaptor) SanitizeTaskResultForAction(_ []byte, action string) []byte {
	a.sanitizeActions = append(a.sanitizeActions, action)
	return []byte(`{"code":"Success","data":{"status":"RUNNING"}}`)
}

func (a *actionAwarePollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func (a *actionAwarePollingAdaptor) ValidateTaskSuccess(context.Context, *model.Task, *relaycommon.TaskInfo) error {
	return a.validationErr
}

func (a *actionAwarePollingAdaptor) ShouldRefundTaskFailure(*model.Task, *relaycommon.TaskInfo) bool {
	return a.refundFailure == nil || *a.refundFailure
}

type pollingContractError struct {
	message   string
	temporary bool
}

func (e *pollingContractError) Error() string   { return e.message }
func (e *pollingContractError) Temporary() bool { return e.temporary }

func TestTaskPollingUsesPersistedActionForAutoDLResultParsing(t *testing.T) {
	truncate(t)
	channel := &model.Channel{
		Id:     9501,
		Type:   constant.ChannelTypeAutoDL,
		Name:   "action-aware AutoDL polling",
		Key:    "autodl-action-aware-key",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	task := &model.Task{
		TaskID:    "task_action_aware_audio",
		Platform:  constant.TaskPlatformAutoDL,
		UserId:    951,
		ChannelId: channel.Id,
		Action:    constant.TaskActionAudioSpeech,
		Status:    model.TaskStatusQueued,
		Progress:  "10%",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-index-tts-task",
			ChannelKeyHash: common.Sha256([]byte(channel.Key)),
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	adaptor := &actionAwarePollingAdaptor{responseBody: []byte(`{
		"code":"Success",
		"data":{"task_id":"provider-index-tts-task","status":"RUNNING"},
		"request_id":"provider-secret"
	}`)}

	err := updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{
		taskPollingMapKey(channel.Id, task.GetUpstreamTaskID()): task,
	})

	require.NoError(t, err)
	assert.Equal(t, constant.TaskActionAudioSpeech, adaptor.fetchAction)
	assert.Equal(t, []string{constant.TaskActionAudioSpeech}, adaptor.parseActions)
	assert.Equal(t, []string{constant.TaskActionAudioSpeech}, adaptor.sanitizeActions)
	assert.Zero(t, adaptor.legacyParses, "action-aware tasks must not fall back to the video-only parser")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.JSONEq(t, `{"code":"Success","data":{"status":"RUNNING"}}`, string(stored.Data))
	assert.NotContains(t, string(stored.Data), "provider-secret")
}

func TestPollTaskOnceReturnsTerminalTaskWithoutProviderAccess(t *testing.T) {
	tests := []struct {
		name   string
		status model.TaskStatus
	}{
		{name: "success", status: model.TaskStatusSuccess},
		{name: "failure", status: model.TaskStatusFailure},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncate(t)
			task := &model.Task{
				TaskID:    "task_poll_terminal_" + test.name,
				Platform:  constant.TaskPlatformAutoDL,
				UserId:    960 + index,
				ChannelId: 999999,
				Action:    constant.TaskActionAudioSpeech,
				Status:    test.status,
				CreatedAt: time.Now().Unix(),
				UpdatedAt: time.Now().Unix(),
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "must-not-be-polled",
				},
			}
			require.NoError(t, model.DB.Create(task).Error)
			factoryCalls := 0
			previousFactory := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
				factoryCalls++
				return &actionAwarePollingAdaptor{}
			}
			t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

			latest, err := PollTaskOnce(context.Background(), task)

			require.NoError(t, err)
			require.NotNil(t, latest)
			assert.Equal(t, task.ID, latest.ID)
			assert.Equal(t, test.status, latest.Status)
			assert.Zero(t, factoryCalls, "terminal tasks must not load a channel or contact the provider")
		})
	}
}

func TestAutoDLAudioPermanentArtifactFailureKeepsAcceptedPerCallCharge(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 971, 972, 973, 100
	const initialQuota = 1000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "autodl-audio-charge-token", initialQuota)
	baseURL := "https://autodl.art"
	channel := &model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeAutoDL,
		Name:    "AutoDL audio charge policy",
		Key:     "autodl-audio-provider-key",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	task := &model.Task{
		TaskID:     "task_autodl_audio_invalid_artifact",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     userID,
		ChannelId:  channelID,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusReserving,
		Quota:      quota,
		Group:      "default",
		SubmitTime: time.Now().Unix(),
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
		Properties: model.Properties{OriginModelName: constant.AutoDLModelIndexTTS2},
		PrivateData: model.TaskPrivateData{
			TokenId:        tokenID,
			ChannelKeyHash: common.Sha256([]byte(channel.Key)),
			BillingContext: &model.TaskBillingContext{PerCallBilling: true, OriginModelName: constant.AutoDLModelIndexTTS2},
		},
	}
	require.NoError(t, model.InsertPreparedTaskBillingReservation(task, nil, &model.ImageBillingReservation{
		TaskID:        task.TaskID,
		RequestID:     "request_autodl_audio_invalid_artifact",
		UserID:        userID,
		TokenID:       tokenID,
		TokenRequired: true,
		ExpectedQuota: quota,
	}))
	require.NoError(t, model.ReserveImageTaskWalletQuota(task.TaskID, userID, quota))
	require.NoError(t, model.ReserveImageTaskTokenQuota(task.TaskID, tokenID, "autodl-audio-charge-token", quota))
	activated, err := model.ActivatePreparedAutoDLTaskCheckpoint(task)
	require.NoError(t, err)
	require.True(t, activated)
	completed, err := task.CompleteSubmissionCheckpoint("provider-index-tts-failed-artifact", []byte(`{"code":"Success","data":{"status":"QUEUED"}}`), quota)
	require.NoError(t, err)
	require.True(t, completed)

	keepCharge := false
	adaptor := &actionAwarePollingAdaptor{
		responseBody: []byte(`{"code":"Success","data":{"status":"SUCCESS"}}`),
		taskResult: &relaycommon.TaskInfo{
			TaskID: "provider-index-tts-failed-artifact",
			Status: model.TaskStatusSuccess,
			Url:    "https://media.example.com/not-a-wave",
		},
		validationErr: &pollingContractError{message: "generated audio is not a valid WAV"},
		refundFailure: &keepCharge,
	}

	err = updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{
		taskPollingMapKey(channel.Id, task.GetUpstreamTaskID()): task,
	})

	require.NoError(t, err)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Contains(t, stored.FailReason, "valid WAV")
	assert.JSONEq(t, `{"code":"GatewayValidationFailed","data":{"status":"FAILURE"}}`, string(stored.Data))
	assert.Equal(t, initialQuota-quota, getUserQuota(t, userID))
	assert.Equal(t, initialQuota-quota, getTokenRemainQuota(t, tokenID))
	reservation, err := model.GetImageBillingReservation(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.ImageBillingReservationActive, reservation.Status)
	assert.Equal(t, quota, reservation.WalletReserved)
	assert.Equal(t, quota, reservation.TokenReserved)
}

func TestAutoDLAudioTemporaryArtifactValidationRemainsRetryable(t *testing.T) {
	truncate(t)
	channel := &model.Channel{Id: 981, Type: constant.ChannelTypeAutoDL, Name: "temporary validation", Key: "temporary-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(channel).Error)
	task := &model.Task{
		TaskID:     "task_autodl_audio_temporary_artifact",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     982,
		ChannelId:  channel.Id,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusQueued,
		SubmitTime: time.Now().Unix(),
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-temporary-artifact",
			ChannelKeyHash: common.Sha256([]byte(channel.Key)),
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	adaptor := &actionAwarePollingAdaptor{
		responseBody: []byte(`{"code":"Success","data":{"status":"SUCCESS"}}`),
		taskResult: &relaycommon.TaskInfo{
			TaskID: "provider-temporary-artifact",
			Status: model.TaskStatusSuccess,
			Url:    "https://media.example.com/temporarily-unavailable.wav",
		},
		validationErr: &pollingContractError{message: "signed URL expired", temporary: true},
	}

	err := updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{
		taskPollingMapKey(channel.Id, task.GetUpstreamTaskID()): task,
	})

	require.ErrorContains(t, err, "validate task result")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), stored.Status)
}
