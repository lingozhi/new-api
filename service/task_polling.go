package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
)

// TaskPollingAdaptor 定义轮询所需的最小适配器接口，避免 service -> relay 的循环依赖
type TaskPollingAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error)
	ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
	// AdjustBillingOnComplete 在任务到达终态（成功/失败）时由轮询循环调用。
	// 返回正数触发差额结算（补扣/退还），返回 0 保持预扣费金额不变。
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}

type contextTaskPollingAdaptor interface {
	FetchTaskWithContext(ctx context.Context, baseURL, key string, body map[string]any, proxy string) (*http.Response, error)
}

type taskResultSanitizer interface {
	SanitizeTaskResult(body []byte) []byte
}

const (
	taskSubmissionReservationTimeout = 3 * time.Minute
	taskSubmissionCheckpointTimeout  = 10 * time.Minute
)

// GetTaskAdaptorFunc 由 main 包注入，用于获取指定平台的任务适配器。
// 打破 service -> relay -> relay/channel -> service 的循环依赖。
var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor

func failTasksWithRefund(ctx context.Context, tasks []*model.Task, reason string) error {
	var combinedErr error
	now := time.Now().Unix()
	for _, task := range tasks {
		if task == nil || task.Status == model.TaskStatusFailure || task.Status == model.TaskStatusSuccess {
			continue
		}
		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		task.FailReason = reason
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		phase := ""
		usageDelta := int64(0)
		if task.Quota != 0 {
			phase = model.BillingAdjustmentPhaseTaskRefund
			usageDelta = -int64(task.Quota)
		}
		won, err := commitTaskTransitionWithBilling(ctx, task, oldStatus, phase, usageDelta)
		if err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("fail task %s: %w", task.TaskID, err))
			continue
		}
		if won && task.Quota != 0 {
			recordTaskQuotaRefund(task, reason)
		}
	}
	return combinedErr
}

// FailTaskWithRefund atomically moves one persisted asynchronous task to a
// terminal failure and records its refund adjustments in the same transaction.
func FailTaskWithRefund(ctx context.Context, task *model.Task, reason string) error {
	return failTasksWithRefund(ctx, []*model.Task{task}, reason)
}

func failTasksWithoutRefund(ctx context.Context, tasks []*model.Task, reason string) error {
	var combinedErr error
	now := time.Now().Unix()
	for _, task := range tasks {
		if task == nil || task.Status == model.TaskStatusFailure || task.Status == model.TaskStatusSuccess {
			continue
		}
		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		task.FailReason = reason
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		won, err := task.UpdateWithStatus(oldStatus)
		if err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("fail task %s without refund: %w", task.TaskID, err))
			continue
		}
		if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s already transitioned, skip ambiguous submission terminalization", task.TaskID))
		}
	}
	return combinedErr
}

// sweepStaleTaskSubmissionReservations refunds AutoDL submissions that never
// reached the provider-call fence. The reservation row owns every debit, so a
// crash before or during pre-consume remains exactly-once recoverable.
func sweepStaleTaskSubmissionReservations(ctx context.Context) {
	cutoff := time.Now().Add(-taskSubmissionReservationTimeout).Unix()
	taskIDs := model.GetStalePreparedTaskBillingReservationIDs(constant.TaskPlatformAutoDL, cutoff, 100)
	if len(taskIDs) == 0 {
		return
	}
	recovered := 0
	for _, taskID := range taskIDs {
		if ctx.Err() != nil {
			return
		}
		applied, err := model.RefundTaskBillingReservation(taskID, "AutoDL 视频提交未开始，已退款")
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweep stale AutoDL submission reservation %s: %v", taskID, err))
			continue
		}
		if applied {
			recovered++
		}
	}
	if recovered > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("swept %d stale AutoDL submission reservation(s)", recovered))
	}
}

// sweepPendingTaskSubmissionRefunds retries explicit provider rejections whose
// durable refund could not finish in the request that observed the rejection.
func sweepPendingTaskSubmissionRefunds(ctx context.Context) {
	tasks := model.GetPendingTaskSubmissionRefunds(constant.TaskPlatformAutoDL, 100)
	for _, task := range tasks {
		reason := task.ProviderError
		if reason == "" {
			reason = "AutoDL video submission was rejected"
		}
		if err := failTasksWithRefund(ctx, []*model.Task{task}, reason); err != nil {
			logger.LogError(ctx, fmt.Sprintf("retry rejected AutoDL submission refund for task %s: %v", task.TaskID, err))
		}
	}
}

// sweepStaleTaskSubmissionCheckpoints resolves the only safe recovery state
// after a crash between an asynchronous provider call and response persistence.
// The upstream outcome is ambiguous and AutoDL has no provider idempotency key,
// so the task is terminalized without an automatic refund. Refunding here would
// let a caller repeatedly disconnect after dispatch while the gateway absorbs
// every accepted provider job.
func sweepStaleTaskSubmissionCheckpoints(ctx context.Context) {
	cutoff := time.Now().Add(-taskSubmissionCheckpointTimeout).Unix()
	tasks := model.GetStaleTaskSubmissionCheckpoints(constant.TaskPlatformAutoDL, cutoff, 100)
	if len(tasks) == 0 {
		return
	}
	reason := "AutoDL 视频提交结果不确定，已停止重试；为避免重复生成未自动退款"
	if err := failTasksWithoutRefund(ctx, tasks, reason); err != nil {
		logger.LogError(ctx, fmt.Sprintf("sweep stale AutoDL submission checkpoints: %v", err))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("swept %d stale AutoDL submission checkpoint(s)", len(tasks)))
}

// sweepTimedOutTasks 在主轮询之前独立清理超时任务。
// 每次最多处理 100 条，剩余的下个周期继续处理。
// 使用 per-task CAS (UpdateWithStatus) 防止覆盖被正常轮询已推进的任务。
func sweepTimedOutTasks(ctx context.Context) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}

	const legacyTaskCutoff int64 = 1740182400 // 2026-02-22 00:00:00 UTC
	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	now := time.Now().Unix()
	timedOutCount := 0

	for _, task := range tasks {
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < legacyTaskCutoff
		isAmbiguousAutoDL := task.Platform == constant.TaskPlatformAutoDL

		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
		} else if isAmbiguousAutoDL {
			task.FailReason = fmt.Sprintf("AutoDL 任务轮询超时（%d分钟）；上游结果不确定，未自动退款", constant.TaskTimeoutMinutes)
		} else {
			task.FailReason = reason
		}
		if isAmbiguousAutoDL {
			won, err := task.UpdateWithStatus(oldStatus)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks terminal transition error for task %s: %v", task.TaskID, err))
				continue
			}
			if !won {
				logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: task %s already transitioned, skip", task.TaskID))
				continue
			}
			timedOutCount++
			continue
		}

		phase := ""
		usageDelta := int64(0)
		if !isLegacy && task.Quota != 0 {
			phase = model.BillingAdjustmentPhaseTaskRefund
			usageDelta = -int64(task.Quota)
		}
		won, err := commitTaskTransitionWithBilling(ctx, task, oldStatus, phase, usageDelta)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks terminal billing transaction error for task %s: %v", task.TaskID, err))
			continue
		}
		if !won {
			logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: task %s already transitioned, skip", task.TaskID))
			continue
		}
		timedOutCount++
		if !isLegacy && task.Quota != 0 {
			recordTaskQuotaRefund(task, reason)
		}
	}

	if timedOutCount > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: timed out %d tasks", timedOutCount))
	}
}

// TaskPollSummary is the result recorded on an async_task_poll system task row,
// summarizing one polling pass.
type TaskPollSummary struct {
	UnfinishedTasks  int `json:"unfinished_tasks"`
	PlatformsScanned int `json:"platforms_scanned"`
	NullTasksFailed  int `json:"null_tasks_failed"`
}

// RunTaskPollingOnce performs one async-task (Suno/video) polling pass
// synchronously. It honors ctx cancellation (the system-task runner cancels it
// when the lease is lost) and, when report is non-nil, reports progress as
// (processedPlatforms, totalPlatforms). It returns immediately if the task
// adaptor factory has not been wired yet, to avoid a nil call during startup.
func RunTaskPollingOnce(ctx context.Context, report func(processed, total int)) TaskPollSummary {
	summary := TaskPollSummary{}
	if GetTaskAdaptorFunc == nil {
		return summary
	}
	if ctx == nil {
		ctx = context.Background()
	}

	common.SysLog("任务进度轮询开始")
	sweepStaleTaskSubmissionReservations(ctx)
	sweepPendingTaskSubmissionRefunds(ctx)
	sweepStaleTaskSubmissionCheckpoints(ctx)
	sweepTimedOutTasks(ctx)
	allTasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	summary.UnfinishedTasks = len(allTasks)
	platformTask := make(map[constant.TaskPlatform][]*model.Task)
	for _, t := range allTasks {
		platformTask[t.Platform] = append(platformTask[t.Platform], t)
	}

	totalPlatforms := len(platformTask)
	processedPlatforms := 0
	for platform, tasks := range platformTask {
		if ctx.Err() != nil {
			break
		}
		if report != nil {
			report(processedPlatforms, totalPlatforms)
		}
		processedPlatforms++
		if len(tasks) == 0 {
			continue
		}
		summary.PlatformsScanned++
		taskChannelM := make(map[int][]string)
		taskM := make(map[string]*model.Task)
		nullTasks := make([]*model.Task, 0)
		for _, task := range tasks {
			upstreamID := task.GetUpstreamTaskID()
			if upstreamID == "" {
				// 统计失败的未完成任务
				nullTasks = append(nullTasks, task)
				continue
			}
			taskM[taskPollingMapKey(task.ChannelId, upstreamID)] = task
			taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], upstreamID)
		}
		if len(nullTasks) > 0 {
			summary.NullTasksFailed += len(nullTasks)
			refundableTasks := make([]*model.Task, 0, len(nullTasks))
			ambiguousAutoDLTasks := make([]*model.Task, 0, len(nullTasks))
			for _, task := range nullTasks {
				if task.Platform == constant.TaskPlatformAutoDL {
					ambiguousAutoDLTasks = append(ambiguousAutoDLTasks, task)
				} else {
					refundableTasks = append(refundableTasks, task)
				}
			}
			err := failTasksWithRefund(ctx, refundableTasks, "upstream task id is missing")
			err = errors.Join(err, failTasksWithoutRefund(ctx, ambiguousAutoDLTasks, "AutoDL upstream task id is missing; no automatic refund was issued"))
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null task_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null task_id task success: %d", len(nullTasks)))
			}
		}
		if len(taskChannelM) == 0 {
			continue
		}

		DispatchPlatformUpdate(ctx, platform, taskChannelM, taskM)
	}
	if report != nil && ctx.Err() == nil {
		report(totalPlatforms, totalPlatforms)
	}
	common.SysLog("任务进度轮询完成")
	return summary
}

func taskPollingMapKey(channelID int, upstreamTaskID string) string {
	return fmt.Sprintf("%d\x00%s", channelID, upstreamTaskID)
}

func taskFromPollingMap(taskM map[string]*model.Task, channelID int, upstreamTaskID string) *model.Task {
	if task := taskM[taskPollingMapKey(channelID, upstreamTaskID)]; task != nil {
		return task
	}
	// Keep direct-call compatibility for focused tests and legacy internal
	// callers that pass a channel-local map.
	return taskM[upstreamTaskID]
}

// DispatchPlatformUpdate 按平台分发轮询更新
func DispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch platform {
	case constant.TaskPlatformMidjourney:
		// MJ 轮询由其自身处理，这里预留入口
	case constant.TaskPlatformSuno:
		_ = UpdateSunoTasks(ctx, taskChannelM, taskM)
	default:
		if err := UpdateVideoTasks(ctx, platform, taskChannelM, taskM); err != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
		}
	}
}

// UpdateSunoTasks 按渠道更新所有 Suno 任务
func UpdateSunoTasks(ctx context.Context, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := updateSunoTasks(ctx, channelId, taskIds, taskM)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新异步任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateSunoTasks(ctx context.Context, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		common.SysLog(fmt.Sprintf("CacheGetChannel: %v", err))
		failedTasks := make([]*model.Task, 0, len(taskIds))
		for _, upstreamID := range taskIds {
			if t := taskFromPollingMap(taskM, channelId, upstreamID); t != nil {
				failedTasks = append(failedTasks, t)
			}
		}
		billingErr := failTasksWithRefund(ctx, failedTasks, fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId))
		if billingErr != nil {
			common.SysLog(fmt.Sprintf("UpdateSunoTask error: %v", billingErr))
		}
		return errors.Join(err, billingErr)
	}
	adaptor := GetTaskAdaptorFunc(constant.TaskPlatformSuno)
	if adaptor == nil {
		return errors.New("adaptor not found")
	}
	proxy := ch.GetSetting().Proxy
	resp, err := adaptor.FetchTask(*ch.BaseURL, ch.Key, map[string]any{
		"ids": taskIds,
	}, proxy)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task Do req error: %v", err))
		return err
	}
	if resp.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
		return fmt.Errorf("Get Task status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Suno Task parse body error: %v", err))
		return err
	}
	var responseItems dto.TaskResponse[[]dto.SunoDataResponse]
	err = common.Unmarshal(responseBody, &responseItems)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Suno Task parse body error2: %v, body: %s", err, string(responseBody)))
		return err
	}
	if !responseItems.IsSuccess() {
		common.SysLog(fmt.Sprintf("渠道 #%d 未完成的任务有: %d, 成功获取到任务数: %s", channelId, len(taskIds), string(responseBody)))
		return err
	}

	for _, responseItem := range responseItems.Data {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		task := taskFromPollingMap(taskM, channelId, responseItem.TaskID)
		if task == nil {
			logger.LogWarn(ctx, fmt.Sprintf("Suno task response ignored: unknown task_id=%s", responseItem.TaskID))
			continue
		}
		if !taskNeedsUpdate(task, responseItem) {
			continue
		}

		oldStatus := task.Status
		task.Status = lo.If(model.TaskStatus(responseItem.Status) != "", model.TaskStatus(responseItem.Status)).Else(task.Status)
		task.FailReason = lo.If(responseItem.FailReason != "", responseItem.FailReason).Else(task.FailReason)
		task.SubmitTime = lo.If(responseItem.SubmitTime != 0, responseItem.SubmitTime).Else(task.SubmitTime)
		task.StartTime = lo.If(responseItem.StartTime != 0, responseItem.StartTime).Else(task.StartTime)
		task.FinishTime = lo.If(responseItem.FinishTime != 0, responseItem.FinishTime).Else(task.FinishTime)
		if task.Status == model.TaskStatusFailure {
			logger.LogInfo(ctx, task.TaskID+" 构建失败，"+task.FailReason)
			task.Progress = "100%"
		}
		if responseItem.Status == model.TaskStatusSuccess {
			task.Progress = "100%"
		}
		task.Data = responseItem.Data

		isTerminal := task.Status == model.TaskStatusFailure || task.Status == model.TaskStatusSuccess
		if isTerminal && task.Status != oldStatus {
			phase := ""
			usageDelta := int64(0)
			if task.Status == model.TaskStatusFailure && task.Quota != 0 {
				phase = model.BillingAdjustmentPhaseTaskRefund
				usageDelta = -int64(task.Quota)
			}
			won, updateErr := commitTaskTransitionWithBilling(ctx, task, oldStatus, phase, usageDelta)
			if updateErr != nil {
				common.SysLog("UpdateSunoTask terminal billing transaction error: " + updateErr.Error())
				continue
			}
			if !won {
				logger.LogWarn(ctx, fmt.Sprintf("Suno task %s already transitioned, skip billing", task.TaskID))
				continue
			}
			if task.Status == model.TaskStatusFailure && task.Quota != 0 {
				recordTaskQuotaRefund(task, task.FailReason)
			}
			continue
		}

		won, updateErr := task.UpdateWithStatus(oldStatus)
		if updateErr != nil {
			common.SysLog("UpdateSunoTask task error: " + updateErr.Error())
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Suno task %s changed concurrently, skip stale update", task.TaskID))
		}
	}
	return nil
}

// taskNeedsUpdate 检查 Suno 任务是否需要更新
func taskNeedsUpdate(oldTask *model.Task, newTask dto.SunoDataResponse) bool {
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if string(oldTask.Status) != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}

	if (oldTask.Status == model.TaskStatusFailure || oldTask.Status == model.TaskStatusSuccess) && oldTask.Progress != "100%" {
		return true
	}

	oldData, _ := common.Marshal(oldTask.Data)
	newData, _ := common.Marshal(newTask.Data)

	sort.Slice(oldData, func(i, j int) bool {
		return oldData[i] < oldData[j]
	})
	sort.Slice(newData, func(i, j int) bool {
		return newData[i] < newData[j]
	})

	if string(oldData) != string(newData) {
		return true
	}
	return false
}

// UpdateVideoTasks 按渠道更新所有视频任务
func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	channelIDs := make([]int, 0, len(taskChannelM))
	for channelID := range taskChannelM {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)

	var wg sync.WaitGroup
	for _, channelId := range channelIDs {
		taskIds := taskChannelM[channelId]
		if len(taskIds) == 0 {
			continue
		}
		taskIds = append([]string(nil), taskIds...)

		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			if err := updateVideoTasks(ctx, platform, channelId, taskIds, taskM); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		if platform == constant.TaskPlatformAutoDL {
			return fmt.Errorf("CacheGetChannel failed for AutoDL channel %d; provider outcome remains unknown: %w", channelId, err)
		}
		failedTasks := make([]*model.Task, 0, len(taskIds))
		for _, upstreamID := range taskIds {
			if t := taskFromPollingMap(taskM, channelId, upstreamID); t != nil {
				failedTasks = append(failedTasks, t)
			}
		}
		billingErr := failTasksWithRefund(ctx, failedTasks, fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId))
		if billingErr != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", billingErr))
		}
		return errors.Join(fmt.Errorf("CacheGetChannel failed: %w", err), billingErr)
	}
	adaptor := GetTaskAdaptorFunc(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	disablePollingSleep := cacheGetChannel.GetOtherSettings().DisableTaskPollingSleep
	for i, taskId := range taskIds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
		if disablePollingSleep || i == len(taskIds)-1 {
			continue
		}

		// sleep 1 second between tasks for this channel only.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskId string, taskM map[string]*model.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	task := taskFromPollingMap(taskM, ch.Id, taskId)
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	isAutoDLTask := task.Platform == constant.TaskPlatformAutoDL
	channelType := ch.Type
	if isAutoDLTask {
		channelType = constant.ChannelTypeAutoDL
	}
	baseURL := constant.ChannelBaseURLs[channelType]
	if ch.GetBaseURL() != "" {
		baseURL = ch.GetBaseURL()
	}
	proxy := ch.GetSetting().Proxy

	key, err := ResolveTaskPollingChannelKey(ch, task.PrivateData)
	if err != nil {
		cause := fmt.Errorf("resolve channel key for task %s: %w", taskId, err)
		return cause
	}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:          channelType,
		ChannelId:            ch.Id,
		ChannelIsMultiKey:    ch.ChannelInfo.IsMultiKey,
		ChannelMultiKeyIndex: task.PrivateData.ChannelMultiKeyIndex,
		ChannelBaseUrl:       baseURL,
		ApiKey:               key,
		ChannelSetting:       ch.GetSetting(),
		ChannelOtherSettings: ch.GetOtherSettings(),
	}})
	fetchBody := map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}
	var resp *http.Response
	if contextAdaptor, ok := adaptor.(contextTaskPollingAdaptor); ok {
		resp, err = contextAdaptor.FetchTaskWithContext(ctx, baseURL, key, fetchBody, proxy)
	} else {
		resp, err = adaptor.FetchTask(baseURL, key, fetchBody, proxy)
	}
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	if resp == nil {
		return fmt.Errorf("fetchTask returned no response for task %s", taskId)
	}
	defer resp.Body.Close()
	responseReader := io.Reader(resp.Body)
	const maxAutoDLPollResponseBytes = 1 << 20
	if isAutoDLTask {
		responseReader = io.LimitReader(resp.Body, maxAutoDLPollResponseBytes+1)
	}
	responseBody, err := io.ReadAll(responseReader)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}
	if isAutoDLTask && len(responseBody) > maxAutoDLPollResponseBytes {
		return fmt.Errorf("AutoDL poll response exceeded 1 MiB for task %s", taskId)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		cause := fmt.Errorf("task %s poll returned HTTP %d", taskId, resp.StatusCode)
		return cause
	}

	if isAutoDLTask {
		logger.LogDebug(ctx, "updateVideoSingleTask AutoDL response: status=%d bytes=%d", resp.StatusCode, len(responseBody))
	} else {
		logger.LogDebug(ctx, "updateVideoSingleTask response: %s", responseBody)
	}

	snap := task.Snapshot()

	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems dto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); !isAutoDLTask && err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, "updateVideoSingleTask parsed as new api response format: %+v", responseItems)
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.GetResultURL()
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		cause := fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
		return cause
	}

	if isAutoDLTask && taskResult.TaskID != task.GetUpstreamTaskID() {
		return fmt.Errorf("AutoDL task identity mismatch: expected %q, got %q", task.GetUpstreamTaskID(), taskResult.TaskID)
	}

	task.Data = redactVideoResponseBody(responseBody)
	if sanitizer, ok := adaptor.(taskResultSanitizer); ok {
		task.Data = sanitizer.SanitizeTaskResult(responseBody)
	}

	if isAutoDLTask {
		logger.LogDebug(ctx, "AutoDL task result parsed: status=%s progress=%s", taskResult.Status, taskResult.Progress)
	} else {
		logger.LogDebug(ctx, "updateVideoSingleTask taskResult: %+v", taskResult)
	}

	now := time.Now().Unix()
	if taskResult.Status == "" {
		//taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
		errorResult := &dto.GeneralErrorResponse{}
		if err = common.Unmarshal(responseBody, &errorResult); err == nil {
			openaiError := errorResult.TryToOpenAIError()
			if openaiError != nil {
				// 返回规范的 OpenAI 错误格式，提取错误信息，判断错误是否为任务失败
				if openaiError.Code == "429" {
					// 429 错误通常表示请求过多或速率限制，暂时不认为是任务失败，保持原状态等待下一轮轮询
					return nil
				}

				// 其他错误认为是任务失败，记录错误信息并更新任务状态
				taskResult = relaycommon.FailTaskInfo("upstream returned error")
			} else {
				// unknown error format, log original response
				logger.LogError(ctx, fmt.Sprintf("Task %s returned empty status with unrecognized error format, response: %s", taskId, string(responseBody)))
				taskResult = relaycommon.FailTaskInfo("upstream returned unrecognized message")
			}
		}
	}

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressSubmitted
	case model.TaskStatusQueued:
		task.Progress = taskcommon.ProgressQueued
	case model.TaskStatusInProgress:
		task.Progress = taskcommon.ProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if strings.HasPrefix(taskResult.Url, "data:") {
			// data: URI (e.g. Vertex base64 encoded video) — keep in Data, not in ResultURL
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if taskResult.Url != "" {
			// Direct upstream URL (e.g. Kling, Ali, Doubao, etc.)
			task.PrivateData.ResultURL = taskResult.Url
		} else {
			// No URL from adaptor — construct proxy URL using public task ID
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
		shouldSettle = true
	case model.TaskStatusFailure:
		logger.LogJson(ctx, fmt.Sprintf("Task %s failed", taskId), task)
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = taskcommon.ProgressComplete
		if quota != 0 {
			shouldRefund = true
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, task.TaskID)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	billingPhase := ""
	billingUsageDelta := int64(0)
	billingReason := ""
	var billingClamp *common.QuotaClamp
	if shouldRefund && quota != 0 {
		billingPhase = model.BillingAdjustmentPhaseTaskRefund
		billingUsageDelta = -int64(quota)
	}
	if shouldSettle {
		actualQuota, reason, clamp, shouldRecalculate := resolveTaskBillingOnComplete(adaptor, task, taskResult)
		if shouldRecalculate {
			billingReason = reason
			billingClamp = clamp
			if actualQuota != quota {
				billingPhase = model.BillingAdjustmentPhaseTaskRecalculate
				billingUsageDelta = int64(actualQuota) - int64(quota)
				task.Quota = actualQuota
			}
		}
	}

	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isDone && snap.Status != task.Status {
		won, err := commitTaskTransitionWithBilling(ctx, task, snap.Status, billingPhase, billingUsageDelta)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("terminal task billing transaction failed for task %s: %s", task.TaskID, err.Error()))
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s already transitioned by another process, skip billing", task.TaskID))
			shouldRefund = false
			shouldSettle = false
		}
	} else if isDone {
		shouldRefund = false
		shouldSettle = false
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update task %s: %s", task.TaskID, err.Error()))
		}
	} else {
		// No changes, skip update
		logger.LogDebug(ctx, "No update needed for task %s", task.TaskID)
	}

	if shouldSettle {
		if billingUsageDelta != 0 {
			recordTaskQuotaRecalculation(ctx, task, quota, billingReason, billingClamp)
		} else if billingReason != "" {
			logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
				task.TaskID, logger.LogQuota(task.Quota), billingReason))
		}
	}
	if shouldRefund {
		recordTaskQuotaRefund(task, task.FailReason)
	}

	return nil
}

// ResolveTaskPollingChannelKey recovers the exact credential selected when an
// asynchronous task was submitted without persisting that credential in cleartext.
func ResolveTaskPollingChannelKey(channel *model.Channel, privateData model.TaskPrivateData) (string, error) {
	if channel == nil {
		return "", errors.New("task channel is required")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return "", errors.New("task channel has no keys")
	}

	if privateData.ChannelKeyHash != "" {
		if privateData.ChannelMultiKeyIndex >= 0 && privateData.ChannelMultiKeyIndex < len(keys) {
			candidate := keys[privateData.ChannelMultiKeyIndex]
			if common.Sha256([]byte(candidate)) == privateData.ChannelKeyHash {
				return candidate, nil
			}
		}
		for _, candidate := range keys {
			if common.Sha256([]byte(candidate)) == privateData.ChannelKeyHash {
				return candidate, nil
			}
		}
		return "", errors.New("task channel key changed after task submission")
	}

	// Compatibility for Gemini/Vertex rows that still carry provider
	// credentials in Key. Polling selection metadata uses ChannelKeyHash.
	if privateData.Key != "" {
		for _, candidate := range keys {
			if candidate == privateData.Key {
				return candidate, nil
			}
		}
		return "", errors.New("legacy task channel key changed after task submission")
	}
	if len(keys) == 1 {
		return keys[0], nil
	}
	return "", errors.New("task channel key selection is missing")
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// resolveTaskBillingOnComplete resolves the final quota before the terminal
// task transition so the transition and its outbox rows can commit atomically.
// 优先级：1. adaptor.AdjustBillingOnComplete 返回正数 → 使用 adaptor 计算的额度
//
//  2. taskResult.TotalTokens > 0 → 按 token 重算
//  3. 都不满足 → 保持预扣额度不变
func resolveTaskBillingOnComplete(adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) (int, string, *common.QuotaClamp, bool) {
	// 0. 按次计费的任务不做差额结算
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		return 0, "", nil, false
	}
	// 1. 优先让 adaptor 决定最终额度
	if actualQuota := adaptor.AdjustBillingOnComplete(task, taskResult); actualQuota > 0 {
		if actualQuota > common.MaxQuota {
			boundedQuota, clamp := common.QuotaFromFloatChecked(float64(actualQuota))
			return boundedQuota, "adaptor计费调整", clamp, true
		}
		return actualQuota, "adaptor计费调整", nil, true
	}
	// 2. 回退到 token 重算
	if taskResult.TotalTokens > 0 {
		return calculateTaskQuotaByTokens(task, taskResult.TotalTokens)
	}
	// 3. 无调整，保持预扣额度
	return 0, "", nil, false
}
