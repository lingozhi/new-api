package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskPollingFetchAdaptor struct {
	mu           sync.Mutex
	taskIDs      []string
	keys         []string
	initKeys     []string
	fetched      chan string
	blockTaskID  string
	blockStarted chan struct{}
	releaseBlock chan struct{}
	blockOnce    sync.Once
}

type autoDLPollingAdaptor struct {
	initKey           string
	legacyFetchCalls  int
	contextFetchCalls int
	responseBody      []byte
	result            *relaycommon.TaskInfo
	parseErr          error
}

func (a *autoDLPollingAdaptor) Init(info *relaycommon.RelayInfo) {
	a.initKey = info.ApiKey
}

func (a *autoDLPollingAdaptor) FetchTask(_ string, _ string, _ map[string]any, _ string) (*http.Response, error) {
	a.legacyFetchCalls++
	return nil, errors.New("legacy FetchTask must not be used for AutoDL polling")
}

func (a *autoDLPollingAdaptor) FetchTaskWithContext(_ context.Context, _ string, _ string, _ map[string]any, _ string) (*http.Response, error) {
	a.contextFetchCalls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(a.responseBody)),
	}, nil
}

func (a *autoDLPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return a.result, a.parseErr
}

func (a *autoDLPollingAdaptor) SanitizeTaskResult([]byte) []byte {
	return []byte(`{"code":"Success","data":{"status":"RUNNING"}}`)
}

func (a *autoDLPollingAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingFetchAdaptor) Init(info *relaycommon.RelayInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.initKeys = append(a.initKeys, info.ApiKey)
}

func (a *taskPollingFetchAdaptor) FetchTask(_ string, key string, body map[string]any, _ string) (*http.Response, error) {
	taskID, _ := body["task_id"].(string)
	if taskID == a.blockTaskID && a.releaseBlock != nil {
		a.blockOnce.Do(func() {
			if a.blockStarted != nil {
				close(a.blockStarted)
			}
		})
		<-a.releaseBlock
	}

	a.mu.Lock()
	a.taskIDs = append(a.taskIDs, taskID)
	a.keys = append(a.keys, key)
	a.mu.Unlock()
	if a.fetched != nil {
		select {
		case a.fetched <- taskID:
		default:
		}
	}

	response := dto.TaskResponse[model.Task]{
		Code: dto.TaskSuccessCode,
		Data: model.Task{
			TaskID:   taskID,
			Status:   model.TaskStatusInProgress,
			Progress: "30%",
		},
	}
	responseBody, err := common.Marshal(response)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}, nil
}

func (a *taskPollingFetchAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
}

func (a *taskPollingFetchAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingFetchAdaptor) fetchCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.taskIDs)
}

func (a *taskPollingFetchAdaptor) fetchedTaskIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.taskIDs...)
}

func (a *taskPollingFetchAdaptor) fetchedKeys() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.keys...)
}

func (a *taskPollingFetchAdaptor) initializedKeys() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.initKeys...)
}

func seedTaskPollingChannel(t *testing.T, id int, disableSleep bool) {
	t.Helper()
	ch := &model.Channel{
		Id:     id,
		Type:   constant.ChannelTypeKling,
		Name:   "polling_channel",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
	}
	if disableSleep {
		ch.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	}
	require.NoError(t, model.DB.Create(ch).Error)
}

func seedPollingTask(t *testing.T, channelID int, publicID string, upstreamID string) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID:    publicID,
		Platform:  constant.TaskPlatform("kling"),
		UserId:    1,
		ChannelId: channelID,
		Action:    constant.TaskActionGenerate,
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamID,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestTaskPollingChannelKeyKeepsSubmittedMultiKeyAfterReorder(t *testing.T) {
	channel := &model.Channel{Key: "key-a\nkey-b"}
	channel.ChannelInfo.IsMultiKey = true
	privateData := model.TaskPrivateData{
		ChannelMultiKeyIndex: 1,
		ChannelKeyHash:       common.Sha256([]byte("key-b")),
	}

	key, err := ResolveTaskPollingChannelKey(channel, privateData)
	require.NoError(t, err)
	assert.Equal(t, "key-b", key)

	channel.Key = "key-b\nkey-a"
	key, err = ResolveTaskPollingChannelKey(channel, privateData)
	require.NoError(t, err)
	assert.Equal(t, "key-b", key)
}

func TestTaskPollingChannelKeyRejectsChangedKey(t *testing.T) {
	channel := &model.Channel{Key: "key-a\nkey-c"}
	channel.ChannelInfo.IsMultiKey = true
	privateData := model.TaskPrivateData{
		ChannelMultiKeyIndex: 1,
		ChannelKeyHash:       common.Sha256([]byte("removed-key")),
	}

	key, err := ResolveTaskPollingChannelKey(channel, privateData)

	require.Error(t, err)
	assert.Empty(t, key)
	assert.Contains(t, err.Error(), "key changed after task submission")
}

func TestTaskPollingChannelKeyUsesExactSingleKey(t *testing.T) {
	channel := &model.Channel{Key: "single-key"}
	privateData := model.TaskPrivateData{ChannelKeyHash: common.Sha256([]byte("single-key"))}

	key, err := ResolveTaskPollingChannelKey(channel, privateData)

	require.NoError(t, err)
	assert.Equal(t, "single-key", key)
}

func TestTaskPollingChannelKeyFingerprintSurvivesCryptoSecretChange(t *testing.T) {
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "process-one-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	channel := &model.Channel{Key: "durable-provider-key"}
	privateData := model.TaskPrivateData{
		ChannelKeyHash: common.Sha256([]byte("durable-provider-key")),
	}
	common.CryptoSecret = "process-two-secret"

	key, err := ResolveTaskPollingChannelKey(channel, privateData)

	require.NoError(t, err)
	assert.Equal(t, "durable-provider-key", key)
}

func TestTaskPollingChannelKeyRejectsLegacyMultiKeyWithoutSelection(t *testing.T) {
	channel := &model.Channel{Key: "key-a\nkey-b"}
	channel.ChannelInfo.IsMultiKey = true

	key, err := ResolveTaskPollingChannelKey(channel, model.TaskPrivateData{})

	require.Error(t, err)
	assert.Empty(t, key)
	assert.Contains(t, err.Error(), "selection is missing")
}

func TestStaleAutoDLSubmissionCheckpointIsFailedWithoutRefundOrResubmit(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 91, 92, 93
	const chargedUserQuota, chargedTokenQuota, preConsumed = 8000, 3000, 2000
	seedUser(t, userID, chargedUserQuota)
	seedToken(t, tokenID, userID, "autodl-checkpoint-token", chargedTokenQuota)
	setTokenUsedQuota(t, tokenID, preConsumed)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Platform = constant.TaskPlatformAutoDL
	task.Status = model.TaskStatusCheckpointPending
	task.Progress = "0%"
	task.SubmitTime = time.Now().Add(-taskSubmissionCheckpointTimeout - time.Second).Unix()
	task.UpdatedAt = task.SubmitTime
	require.NoError(t, model.DB.Create(task).Error)

	sweepStaleTaskSubmissionCheckpoints(context.Background())

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, stored.Status)
	assert.Equal(t, "100%", stored.Progress)
	assert.Contains(t, stored.FailReason, "未自动退款")
	assert.Equal(t, chargedUserQuota, getUserQuota(t, userID))
	assert.Equal(t, chargedTokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, getTokenUsedQuota(t, tokenID))
}

func TestRejectedAutoDLSubmissionRefundIsRecoveredBySweep(t *testing.T) {
	truncate(t)

	const userID, tokenID, quota = 94, 95, 700
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "autodl-rejected-token", 3000)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_autodl_rejected_refund",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     userID,
		Status:     model.TaskStatusReserving,
		Progress:   "0%",
		Quota:      quota,
		SubmitTime: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, model.InsertPreparedTaskBillingReservation(task, nil, &model.ImageBillingReservation{
		TaskID:        task.TaskID,
		RequestID:     "request_autodl_rejected_refund",
		UserID:        userID,
		TokenID:       tokenID,
		TokenRequired: true,
		ExpectedQuota: quota,
	}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RequestId:                "request_autodl_rejected_refund",
		UserId:                   userID,
		TokenId:                  tokenID,
		TokenKey:                 "autodl-rejected-token",
		OriginModelName:          "MiniMax-H3",
		ForcePreConsume:          true,
		BillingReservationTaskID: task.TaskID,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(c, quota, info))
	activated, err := model.ActivatePreparedAutoDLTaskCheckpoint(task)
	require.NoError(t, err)
	require.True(t, activated)
	marked, err := task.MarkSubmissionRejected("AutoDL video submission was rejected")
	require.NoError(t, err)
	require.True(t, marked)

	// Simulate the request-local refund attempt failing after the durable marker
	// was written. The next sweep reloads the authoritative task and retries.
	requestCopy := *task
	requestCopy.UserId++
	require.Error(t, FailTaskWithRefund(context.Background(), &requestCopy, task.ProviderError))
	assert.Equal(t, 5000-quota, getUserQuota(t, userID))
	assert.Equal(t, 3000-quota, getTokenRemainQuota(t, tokenID))

	sweepPendingTaskSubmissionRefunds(context.Background())

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Equal(t, "AutoDL video submission was rejected", stored.FailReason)
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, 3000, getTokenRemainQuota(t, tokenID))
	reservation, err := model.GetImageBillingReservation(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.ImageBillingReservationRefunded, reservation.Status)
}

func TestFreshAutoDLBillingReservationIsInvisibleToNormalPolling(t *testing.T) {
	truncate(t)

	const userID, tokenID = 111, 112
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "autodl-fresh-reservation-token", 3000)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_autodl_fresh_reservation",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     userID,
		Status:     model.TaskStatusReserving,
		Progress:   "0%",
		Quota:      500,
		SubmitTime: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, model.InsertPreparedTaskBillingReservation(task, nil, &model.ImageBillingReservation{
		TaskID:        task.TaskID,
		RequestID:     "request_autodl_fresh_reservation",
		UserID:        userID,
		TokenID:       tokenID,
		TokenRequired: true,
		ExpectedQuota: task.Quota,
	}))

	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &autoDLPollingAdaptor{} }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	summary, err := RunTaskPollingOnce(context.Background(), nil)
	require.NoError(t, err)

	assert.Zero(t, summary.UnfinishedTasks)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusReserving, stored.Status)
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, 3000, getTokenRemainQuota(t, tokenID))
	var adjustmentCount int64
	require.NoError(t, model.DB.Model(&model.BillingAdjustmentOutbox{}).Count(&adjustmentCount).Error)
	assert.Zero(t, adjustmentCount)
}

func TestRunTaskPollingOnceReturnsPlatformFailures(t *testing.T) {
	truncate(t)
	previousTaskQueryLimit := constant.TaskQueryLimit
	constant.TaskQueryLimit = 100
	t.Cleanup(func() { constant.TaskQueryLimit = previousTaskQueryLimit })

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_poll_missing_channel",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     1,
		ChannelId:  9999,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusInProgress,
		Progress:   "30%",
		SubmitTime: now,
		CreatedAt:  now,
		UpdatedAt:  now,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream_missing_channel",
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.Len(t, model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit), 1)

	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &autoDLPollingAdaptor{} }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	summary, err := RunTaskPollingOnce(context.Background(), nil)

	require.Error(t, err)
	assert.Equal(t, 1, summary.UnfinishedTasks)
	assert.Equal(t, 1, summary.PlatformsScanned)
	assert.Equal(t, 1, summary.PlatformFailures)
}

func TestTimedOutAutoDLTaskFailsWithoutAutomaticRefund(t *testing.T) {
	truncate(t)

	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })

	const userID, quota = 113, 700
	seedUser(t, userID, 5000)
	task := makeTask(userID, 0, quota, 0, BillingSourceWallet, 0)
	task.TaskID = "task_autodl_poll_timeout"
	task.Platform = constant.TaskPlatformAutoDL
	task.SubmitTime = time.Now().Add(-2 * time.Minute).Unix()
	task.CreatedAt = task.SubmitTime
	task.UpdatedAt = task.SubmitTime
	require.NoError(t, model.DB.Create(task).Error)

	sweepTimedOutTasks(context.Background())

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Contains(t, stored.FailReason, "未自动退款")
	assert.Equal(t, 5000, getUserQuota(t, userID))
}

func TestStaleAutoDLBillingReservationRefundsRecordedDebits(t *testing.T) {
	truncate(t)

	const userID, tokenID, quota = 121, 122, 700
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "autodl-stale-reservation-token", 3000)
	now := time.Now().Add(-taskSubmissionReservationTimeout - time.Second).Unix()
	task := &model.Task{
		TaskID:     "task_autodl_stale_reservation",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     userID,
		Status:     model.TaskStatusReserving,
		Progress:   "0%",
		Quota:      quota,
		SubmitTime: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, model.InsertPreparedTaskBillingReservation(task, nil, &model.ImageBillingReservation{
		TaskID:        task.TaskID,
		RequestID:     "request_autodl_stale_reservation",
		UserID:        userID,
		TokenID:       tokenID,
		TokenRequired: true,
		ExpectedQuota: quota,
	}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RequestId:                "request_autodl_stale_reservation",
		UserId:                   userID,
		TokenId:                  tokenID,
		TokenKey:                 "autodl-stale-reservation-token",
		OriginModelName:          "MiniMax-H3",
		ForcePreConsume:          true,
		BillingReservationTaskID: task.TaskID,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(c, quota, info))
	assert.Equal(t, 5000-quota, getUserQuota(t, userID))
	assert.Equal(t, 3000-quota, getTokenRemainQuota(t, tokenID))

	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &autoDLPollingAdaptor{} }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	_, err := RunTaskPollingOnce(context.Background(), nil)
	require.NoError(t, err)

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, stored.Status)
	assert.Equal(t, "100%", stored.Progress)
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, 3000, getTokenRemainQuota(t, tokenID))
	reservation, err := model.GetImageBillingReservation(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.ImageBillingReservationRefunded, reservation.Status)
}

func TestUpdateVideoTasksNeverPassesMultiKeyBlobToAdaptor(t *testing.T) {
	truncate(t)

	const channelID = 103
	channel := &model.Channel{
		Id:     channelID,
		Type:   constant.ChannelTypeKling,
		Name:   "multi_key_polling_channel",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
		},
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, model.DB.Create(channel).Error)

	task := seedPollingTask(t, channelID, "task_public_multikey", "upstream_multikey")
	task.PrivateData.ChannelMultiKeyIndex = 1
	task.PrivateData.ChannelKeyHash = common.Sha256([]byte("key-b"))
	require.NoError(t, model.DB.Model(task).Update("private_data", task.PrivateData).Error)

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	err := UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
		channelID: {task.GetUpstreamTaskID()},
	}, map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"key-b"}, adaptor.initializedKeys())
	assert.Equal(t, []string{"key-b"}, adaptor.fetchedKeys())
}

func TestUpdateVideoTasksSeparatesSameUpstreamIDAcrossChannels(t *testing.T) {
	truncate(t)

	const firstChannelID, secondChannelID = 104, 105
	const sharedUpstreamID = "shared-upstream-id"
	seedTaskPollingChannel(t, firstChannelID, true)
	seedTaskPollingChannel(t, secondChannelID, true)
	first := seedPollingTask(t, firstChannelID, "task_shared_upstream_first", sharedUpstreamID)
	second := seedPollingTask(t, secondChannelID, "task_shared_upstream_second", sharedUpstreamID)
	for _, task := range []*model.Task{first, second} {
		task.Status = model.TaskStatusQueued
		task.Progress = taskcommon.ProgressQueued
		require.NoError(t, model.DB.Model(task).Updates(map[string]any{
			"status":   task.Status,
			"progress": task.Progress,
		}).Error)
	}

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	err := UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
		firstChannelID:  {sharedUpstreamID},
		secondChannelID: {sharedUpstreamID},
	}, map[string]*model.Task{
		taskPollingMapKey(firstChannelID, sharedUpstreamID):  first,
		taskPollingMapKey(secondChannelID, sharedUpstreamID): second,
	})

	require.NoError(t, err)
	var storedFirst, storedSecond model.Task
	require.NoError(t, model.DB.First(&storedFirst, first.ID).Error)
	require.NoError(t, model.DB.First(&storedSecond, second.ID).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, storedFirst.Status)
	assert.EqualValues(t, model.TaskStatusInProgress, storedSecond.Status)
	assert.Equal(t, taskcommon.ProgressInProgress, storedFirst.Progress)
	assert.Equal(t, taskcommon.ProgressInProgress, storedSecond.Progress)
}

func TestUpdateVideoTasksDefaultSleepWaitsBetweenTasks(t *testing.T) {
	truncate(t)

	const channelID = 101
	seedTaskPollingChannel(t, channelID, false)
	first := seedPollingTask(t, channelID, "task_public_1", "upstream_1")
	second := seedPollingTask(t, channelID, "task_public_2", "upstream_2")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		channelID: {
			first.GetUpstreamTaskID(),
			second.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		first.GetUpstreamTaskID():  first,
		second.GetUpstreamTaskID(): second,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, adaptor.fetchCount())
}

func TestUpdateVideoTasksCanSkipPollingSleepPerChannel(t *testing.T) {
	truncate(t)

	const channelID = 102
	seedTaskPollingChannel(t, channelID, true)
	first := seedPollingTask(t, channelID, "task_public_3", "upstream_3")
	second := seedPollingTask(t, channelID, "task_public_4", "upstream_4")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		channelID: {
			first.GetUpstreamTaskID(),
			second.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		first.GetUpstreamTaskID():  first,
		second.GetUpstreamTaskID(): second,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, adaptor.fetchCount())
}

func TestUpdateVideoTasksDefaultSleepDoesNotBlockOtherChannels(t *testing.T) {
	truncate(t)

	const firstChannelID = 201
	const secondChannelID = 202
	seedTaskPollingChannel(t, firstChannelID, false)
	seedTaskPollingChannel(t, secondChannelID, false)
	firstChannelFirst := seedPollingTask(t, firstChannelID, "task_public_5", "upstream_a_1")
	firstChannelSecond := seedPollingTask(t, firstChannelID, "task_public_6", "upstream_a_2")
	secondChannelFirst := seedPollingTask(t, secondChannelID, "task_public_7", "upstream_b_1")
	secondChannelSecond := seedPollingTask(t, secondChannelID, "task_public_8", "upstream_b_2")

	adaptor := &taskPollingFetchAdaptor{fetched: make(chan string, 4)}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	gopool.Go(func() {
		errCh <- UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
			firstChannelID: {
				firstChannelFirst.GetUpstreamTaskID(),
				firstChannelSecond.GetUpstreamTaskID(),
			},
			secondChannelID: {
				secondChannelFirst.GetUpstreamTaskID(),
				secondChannelSecond.GetUpstreamTaskID(),
			},
		}, map[string]*model.Task{
			firstChannelFirst.GetUpstreamTaskID():   firstChannelFirst,
			firstChannelSecond.GetUpstreamTaskID():  firstChannelSecond,
			secondChannelFirst.GetUpstreamTaskID():  secondChannelFirst,
			secondChannelSecond.GetUpstreamTaskID(): secondChannelSecond,
		})
	})

	firstPolls := make([]string, 0, 2)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for len(firstPolls) < 2 {
		select {
		case taskID := <-adaptor.fetched:
			firstPolls = append(firstPolls, taskID)
		case <-deadline.C:
			cancel()
			t.Fatal("both channels did not start their first poll")
		}
	}
	cancel()

	require.ErrorIs(t, <-errCh, context.Canceled)
	assert.ElementsMatch(t, []string{"upstream_a_1", "upstream_b_1"}, firstPolls)
	assert.ElementsMatch(t, []string{"upstream_a_1", "upstream_b_1"}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels(t *testing.T) {
	truncate(t)

	const slowChannelID = 251
	const fastChannelID = 252
	seedTaskPollingChannel(t, slowChannelID, false)
	seedTaskPollingChannel(t, fastChannelID, true)
	slowTask := seedPollingTask(t, slowChannelID, "task_public_slow", "upstream_slow_1")
	fastFirst := seedPollingTask(t, fastChannelID, "task_public_fast_1", "upstream_fast_parallel_1")
	fastSecond := seedPollingTask(t, fastChannelID, "task_public_fast_2", "upstream_fast_parallel_2")
	slowUpstreamID := slowTask.GetUpstreamTaskID()
	fastFirstUpstreamID := fastFirst.GetUpstreamTaskID()
	fastSecondUpstreamID := fastSecond.GetUpstreamTaskID()

	adaptor := &taskPollingFetchAdaptor{
		fetched:      make(chan string, 4),
		blockTaskID:  slowUpstreamID,
		blockStarted: make(chan struct{}),
		releaseBlock: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseBlockedTask := func() {
		releaseOnce.Do(func() {
			close(adaptor.releaseBlock)
		})
	}
	t.Cleanup(releaseBlockedTask)
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	errCh := make(chan error, 1)
	gopool.Go(func() {
		errCh <- UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
			slowChannelID: {
				slowUpstreamID,
			},
			fastChannelID: {
				fastFirstUpstreamID,
				fastSecondUpstreamID,
			},
		}, map[string]*model.Task{
			slowUpstreamID:       slowTask,
			fastFirstUpstreamID:  fastFirst,
			fastSecondUpstreamID: fastSecond,
		})
	})

	select {
	case <-adaptor.blockStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("slow channel did not start blocking")
	}

	require.Eventually(t, func() bool {
		fetchedTaskIDs := adaptor.fetchedTaskIDs()
		return len(fetchedTaskIDs) == 2 &&
			fetchedTaskIDs[0] == fastFirstUpstreamID &&
			fetchedTaskIDs[1] == fastSecondUpstreamID
	}, 500*time.Millisecond, 10*time.Millisecond)

	releaseBlockedTask()
	require.NoError(t, <-errCh)
	assert.ElementsMatch(t, []string{
		slowUpstreamID,
		fastFirstUpstreamID,
		fastSecondUpstreamID,
	}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksMixedChannelSleepSettings(t *testing.T) {
	truncate(t)

	const sleepyChannelID = 301
	const fastChannelID = 302
	seedTaskPollingChannel(t, sleepyChannelID, false)
	seedTaskPollingChannel(t, fastChannelID, true)
	sleepyFirst := seedPollingTask(t, sleepyChannelID, "task_public_9", "upstream_sleepy_1")
	sleepySecond := seedPollingTask(t, sleepyChannelID, "task_public_10", "upstream_sleepy_2")
	fastFirst := seedPollingTask(t, fastChannelID, "task_public_11", "upstream_fast_1")
	fastSecond := seedPollingTask(t, fastChannelID, "task_public_12", "upstream_fast_2")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		sleepyChannelID: {
			sleepyFirst.GetUpstreamTaskID(),
			sleepySecond.GetUpstreamTaskID(),
		},
		fastChannelID: {
			fastFirst.GetUpstreamTaskID(),
			fastSecond.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		sleepyFirst.GetUpstreamTaskID():  sleepyFirst,
		sleepySecond.GetUpstreamTaskID(): sleepySecond,
		fastFirst.GetUpstreamTaskID():    fastFirst,
		fastSecond.GetUpstreamTaskID():   fastSecond,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ElementsMatch(t, []string{"upstream_sleepy_1", "upstream_fast_1", "upstream_fast_2"}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoSingleTaskUsesContextFetchAndSanitizesAutoDLData(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:     401,
		Type:   constant.ChannelTypeAutoDL,
		Name:   "autodl_polling_channel",
		Key:    "autodl-selected-key",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	task := &model.Task{
		TaskID:    "task_public_autodl",
		Platform:  constant.TaskPlatform("60"),
		UserId:    1,
		ChannelId: channel.Id,
		Status:    model.TaskStatusQueued,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-secret",
			ChannelKeyHash: common.Sha256([]byte(channel.Key)),
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	adaptor := &autoDLPollingAdaptor{
		responseBody: []byte(`{"code":"success","data":{"task_id":"upstream-secret","status":"RUNNING"},"request_id":"provider-secret"}`),
		result: &relaycommon.TaskInfo{
			TaskID:   "upstream-secret",
			Status:   model.TaskStatusInProgress,
			Progress: "30%",
		},
	}
	err := updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	})
	require.NoError(t, err)

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, "autodl-selected-key", adaptor.initKey)
	assert.Zero(t, adaptor.legacyFetchCalls)
	assert.Equal(t, 1, adaptor.contextFetchCalls)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.JSONEq(t, `{"code":"Success","data":{"status":"RUNNING"}}`, string(stored.Data))
	assert.NotContains(t, string(stored.Data), "upstream-secret")
	assert.NotContains(t, string(stored.Data), "provider-secret")
}

func TestUpdateVideoSingleTaskRejectsMismatchedAutoDLTaskID(t *testing.T) {
	truncate(t)

	channel := &model.Channel{
		Id:     402,
		Type:   constant.ChannelTypeAutoDL,
		Name:   "autodl_identity_channel",
		Key:    "autodl-selected-key",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	task := &model.Task{
		TaskID:    "task_public_identity",
		Platform:  constant.TaskPlatform("60"),
		UserId:    1,
		ChannelId: channel.Id,
		Status:    model.TaskStatusQueued,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-expected",
			ChannelKeyHash: common.Sha256([]byte(channel.Key)),
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	adaptor := &autoDLPollingAdaptor{
		responseBody: []byte(`{"code":"Success","data":{"task_id":"upstream-other","status":"RUNNING"}}`),
		result: &relaycommon.TaskInfo{
			TaskID:   "upstream-other",
			Status:   model.TaskStatusInProgress,
			Progress: "30%",
		},
	}
	err := updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	})
	require.ErrorContains(t, err, "identity mismatch")

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, 1, adaptor.contextFetchCalls)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), stored.Status)
	assert.Empty(t, stored.FailReason)
}
