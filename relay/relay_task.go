package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	//PerCallPrice   types.PriceData
}

type TaskSubmissionCheckpoint struct {
	Prepare  func(platform constant.TaskPlatform, expectedQuota int) *dto.TaskError
	Activate func(preConsumedQuota int) *dto.TaskError
}

type taskDispatchPreflighter interface {
	PreflightDispatch(c *gin.Context, info *relaycommon.RelayInfo) error
}

const autoDLSubmissionTimeout = 2 * time.Minute

const autoDLResultRefreshTTL = 30 * time.Second

const autoDLTaskQueryWindow = 7 * 24 * time.Hour

var autoDLResultRefreshGroup singleflight.Group

var getTaskAdaptorForResultRefresh = GetTaskAdaptor

// IsAutoDLTaskWithinQueryWindow applies the MiniMax-compatible seven-day
// lookup window to every public route that can expose or refresh an AutoDL
// result.
func IsAutoDLTaskWithinQueryWindow(task *model.Task, now time.Time) bool {
	if task == nil || task.Platform != constant.TaskPlatformAutoDL {
		return false
	}
	taskTimestamp := task.SubmitTime
	if taskTimestamp == 0 {
		taskTimestamp = task.CreatedAt
	}
	return taskTimestamp > 0 && taskTimestamp >= now.Add(-autoDLTaskQueryWindow).Unix()
}

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			// 历史任务数据可能包含未经校验的时长，作为计费乘数前必须钳制
			if seconds > relaycommon.MaxTaskDurationSeconds {
				seconds = relaycommon.MaxTaskDurationSeconds
			}
			sizeStr, _ := taskData["size"].(string)
			info.PriceData.AddOtherRatio("seconds", float64(seconds))
			info.PriceData.AddOtherRatio("size", 1)
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.AddOtherRatio("size", 1.666667)
			}
		}
	}

	return nil
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 构建请求 → 持久化计费预留 →
// 预扣费（仅首次）→ 激活 provider-call checkpoint → 发送/解析上游请求
// → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo, checkpoint *TaskSubmissionCheckpoint) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度（饱和转换，防止溢出成负数）
	if !common.StringsContains(constant.TaskPricePatches, modelName) {
		quotaWithRatios := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota))
		quota, clamp := common.QuotaFromFloatChecked(quotaWithRatios)
		info.PriceData.Quota = quota
		noteTaskQuotaClamp(info, clamp)
	}
	// A configured paid task must never collapse to a free request through
	// integer truncation. Besides undercharging, zero makes subscription billing
	// reserve its mandatory one-unit minimum while the task later tries to
	// settle back to zero after its reservation has become active.
	if !info.PriceData.FreeModel && info.PriceData.Quota == 0 {
		info.PriceData.Quota = 1
	}

	// 7. 先构建请求体。任何本地转换失败都应发生在持久化计费预留和
	// 扣费之前。
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}
	// Advance the provider-call fence only after every deterministic local URL,
	// header, and proxy configuration check has succeeded.
	if preflighter, ok := adaptor.(taskDispatchPreflighter); ok {
		if err := preflighter.PreflightDispatch(c, info); err != nil {
			return nil, service.TaskErrorWrapper(err, "dispatch_preflight_failed", http.StatusInternalServerError)
		}
	}
	if checkpoint != nil && checkpoint.Prepare != nil {
		if taskErr := checkpoint.Prepare(platform, info.PriceData.Quota); taskErr != nil {
			return nil, taskErr
		}
	}

	// 8. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 9. 将已记录的扣费原子转交给 provider-call checkpoint。只有这一步
	// 成功后才允许发起不可幂等的 AutoDL 请求。
	if checkpoint != nil && c.Request.Context().Err() != nil {
		return nil, service.TaskErrorWrapper(
			c.Request.Context().Err(),
			"client_disconnected_before_dispatch",
			http.StatusRequestTimeout,
		)
	}
	if checkpoint != nil && checkpoint.Activate != nil {
		if taskErr := checkpoint.Activate(info.FinalPreConsumedQuota); taskErr != nil {
			return nil, taskErr
		}
	}

	// 10. AutoDL 没有提交幂等键；总 deadline 必须严格短于 stale
	// checkpoint 回收窗口，避免仍在进行的请求被另一个 worker 退款。
	originalRequest := c.Request
	isAutoDLWorkflowSubmission := info.ChannelType == constant.ChannelTypeAutoDL &&
		constant.AutoDLSupportsRequest(c.Request.URL.Path, info.OriginModelName)
	if isAutoDLWorkflowSubmission {
		// Once the durable checkpoint is active, a client disconnect must not
		// cancel an already charged asynchronous submission. Preserve request
		// values while bounding the detached provider call to two minutes.
		submitCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), autoDLSubmissionTimeout)
		c.Request = c.Request.WithContext(submitCtx)
		defer func() {
			c.Request = originalRequest
			cancel()
		}()
	}

	// 11. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, service.TaskErrorWrapper(errors.New("upstream returned no response"), "empty_upstream_response", http.StatusBadGateway)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		if isAutoDLWorkflowSubmission {
			// AutoDL workflow routes never expose the upstream error body. Preserve the
			// already-known HTTP result before touching a body that may be truncated
			// or reset; the controller needs this status to decide whether the
			// provider definitively rejected the request or may have created a task.
			return nil, service.TaskErrorWrapper(
				fmt.Errorf("generation service rejected the request (HTTP %d)", resp.StatusCode),
				"fail_to_fetch_task",
				resp.StatusCode,
			)
		}
		const maxTaskErrorBodyBytes = 64 << 10
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxTaskErrorBodyBytes+1))
		if readErr != nil {
			return nil, service.TaskErrorWrapper(
				fmt.Errorf("read upstream error response with status %d: %w", resp.StatusCode, readErr),
				"read_upstream_error_failed",
				http.StatusBadGateway,
			)
		}
		if len(responseBody) > maxTaskErrorBodyBytes {
			responseBody = responseBody[:maxTaskErrorBodyBytes]
		}
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 12. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios()
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 13. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 14. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		if adjustedQuota, ok := recalcQuotaFromRatios(info, adjustedRatios); ok {
			// 基于调整后的 ratios 重新计算 quota
			finalQuota = adjustedQuota
			info.PriceData.ReplaceOtherRatios(adjustedRatios)
			info.PriceData.Quota = finalQuota
		}
	}

	return &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.RemoveOtherRatiosFromFloat(float64(info.PriceData.Quota))
	priceData := info.PriceData
	if !priceData.ReplaceOtherRatios(ratios) {
		return 0, false
	}
	// 应用新的 ratios
	result := priceData.ApplyOtherRatiosToFloat(baseQuota)
	quota, clamp := common.QuotaFromFloatChecked(result)
	noteTaskQuotaClamp(info, clamp)
	return quota, true
}

// noteTaskQuotaClamp records the first quota saturation event onto the task's
// RelayInfo so LogTaskConsumption can surface it on the submit log's
// admin_info. First non-nil clamp wins.
func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || info == nil {
		return
	}
	if info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		return service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")
	isMiniMaxVideoV2API := strings.HasPrefix(c.Request.URL.Path, "/v2/query/video_generation/")

	if isMiniMaxVideoV2API {
		if originTask.Platform != constant.TaskPlatformAutoDL ||
			originTask.Action != constant.TaskActionVideoGenerationV2 ||
			!IsAutoDLTaskWithinQueryWindow(originTask, time.Now()) {
			taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
			return
		}
		RefreshAutoDLSuccessTask(c.Request.Context(), originTask)
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		converter, ok := adaptor.(channel.MiniMaxVideoV2Converter)
		if !ok {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
			return
		}
		respBody, err = converter.ConvertToMiniMaxVideoV2(originTask)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "convert_to_minimax_video_v2_failed", http.StatusInternalServerError)
		}
		return
	}

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// RefreshAutoDLSuccessTask re-queries a completed AutoDL workflow so callers
// receive a fresh signed result URL after the previously stored URL expires.
// It never changes billing or terminal status and falls back to the stored URL
// when the refresh cannot be completed safely.
func RefreshAutoDLSuccessTask(ctx context.Context, task *model.Task) {
	if task == nil || task.Status != model.TaskStatusSuccess || !IsAutoDLTaskWithinQueryWindow(task, time.Now()) {
		return
	}
	now := time.Now().Unix()
	refreshTTLSeconds := int64(autoDLResultRefreshTTL / time.Second)
	if task.PrivateData.ResultRefreshedAt > 0 &&
		task.PrivateData.ResultRefreshedAt <= now &&
		now-task.PrivateData.ResultRefreshedAt < refreshTTLSeconds {
		return
	}

	value, err, _ := autoDLResultRefreshGroup.Do(task.TaskID, func() (any, error) {
		latest, exists, err := model.GetByTaskId(task.UserId, task.TaskID)
		if err != nil || !exists || latest == nil {
			return nil, err
		}
		if latest.Status != model.TaskStatusSuccess {
			return latest, nil
		}
		if !IsAutoDLTaskWithinQueryWindow(latest, time.Now()) {
			return latest, nil
		}
		refreshAt := time.Now().Unix()
		if latest.PrivateData.ResultRefreshedAt > 0 &&
			latest.PrivateData.ResultRefreshedAt <= refreshAt &&
			refreshAt-latest.PrivateData.ResultRefreshedAt < refreshTTLSeconds {
			return latest, nil
		}
		claimed, err := latest.ClaimSuccessResultRefresh(refreshAt-refreshTTLSeconds, refreshAt)
		if err != nil || !claimed {
			return latest, err
		}

		channelModel, err := model.GetChannelById(latest.ChannelId, true)
		if err != nil || channelModel == nil {
			return latest, err
		}
		key, err := service.ResolveTaskPollingChannelKey(channelModel, latest.PrivateData)
		if err != nil {
			return latest, err
		}
		adaptor := getTaskAdaptorForResultRefresh(latest.Platform)
		if adaptor == nil {
			return latest, errors.New("AutoDL task adaptor is unavailable")
		}
		baseURL := channelModel.GetBaseURL()
		if baseURL == "" {
			baseURL = constant.ChannelBaseURLs[constant.ChannelTypeAutoDL]
		}
		adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAutoDL,
			ChannelId:            channelModel.Id,
			ChannelIsMultiKey:    channelModel.ChannelInfo.IsMultiKey,
			ChannelMultiKeyIndex: latest.PrivateData.ChannelMultiKeyIndex,
			ChannelBaseUrl:       baseURL,
			ApiKey:               key,
			ChannelSetting:       channelModel.GetSetting(),
			ChannelOtherSettings: channelModel.GetOtherSettings(),
		}})
		fetchBody := map[string]any{
			"task_id": latest.GetUpstreamTaskID(),
			"action":  latest.Action,
		}
		if ctx == nil {
			ctx = context.Background()
		}
		refreshCtx, cancel := context.WithTimeout(ctx, autoDLResultRefreshTTL)
		defer cancel()
		var resp *http.Response
		if contextAdaptor, ok := adaptor.(interface {
			FetchTaskWithContext(context.Context, string, string, map[string]any, string) (*http.Response, error)
		}); ok {
			resp, err = contextAdaptor.FetchTaskWithContext(refreshCtx, baseURL, key, fetchBody, channelModel.GetSetting().Proxy)
		} else {
			resp, err = adaptor.FetchTask(baseURL, key, fetchBody, channelModel.GetSetting().Proxy)
		}
		if err != nil {
			return latest, err
		}
		if resp == nil {
			return latest, errors.New("AutoDL task refresh returned no response")
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return latest, fmt.Errorf("AutoDL task refresh returned HTTP %d", resp.StatusCode)
		}
		const maxAutoDLRefreshResponseBytes = 1 << 20
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxAutoDLRefreshResponseBytes+1))
		if err != nil {
			return latest, err
		}
		if len(body) > maxAutoDLRefreshResponseBytes {
			return latest, errors.New("AutoDL task refresh response exceeded the 1 MiB limit")
		}
		var result *relaycommon.TaskInfo
		if parser, ok := adaptor.(interface {
			ParseTaskResultForAction([]byte, string) (*relaycommon.TaskInfo, error)
		}); ok {
			result, err = parser.ParseTaskResultForAction(body, latest.Action)
		} else {
			result, err = adaptor.ParseTaskResult(body)
		}
		if err != nil {
			return latest, err
		}
		if result == nil || result.TaskID != latest.GetUpstreamTaskID() || result.Status != model.TaskStatusSuccess || result.Url == "" {
			return latest, errors.New("AutoDL task refresh returned an unexpected result")
		}

		data := latest.Data
		if sanitizer, ok := adaptor.(interface {
			SanitizeTaskResultForAction([]byte, string) []byte
		}); ok {
			data = sanitizer.SanitizeTaskResultForAction(body, latest.Action)
		} else if sanitizer, ok := adaptor.(interface{ SanitizeTaskResult([]byte) []byte }); ok {
			data = sanitizer.SanitizeTaskResult(body)
		}
		if _, err := latest.RefreshSuccessResult(result.Url, data, refreshAt); err != nil {
			return latest, err
		}
		return latest, nil
	})
	if err != nil {
		common.SysError(fmt.Sprintf("refresh AutoDL task result failed: task_id=%s channel_id=%d error=%v", task.TaskID, task.ChannelId, err))
		return
	}
	if refreshed, ok := value.(*model.Task); ok && refreshed != nil {
		*task = *refreshed
	}
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func taskModel2Dto(task *model.Task, includeInternalRouting bool) *dto.TaskDto {
	resultURL := ""
	if task.Status == model.TaskStatusSuccess {
		resultURL = task.GetResultURL()
	}
	taskDto := &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  resultURL,
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.Data,
	}
	if includeInternalRouting || task.Platform != constant.TaskPlatformAutoDL {
		return taskDto
	}

	properties := task.Properties
	properties.UpstreamModelName = ""
	taskDto.Properties = properties
	taskDto.ChannelId = 0
	taskDto.Data = nil
	taskDto.FailReason = ""
	if task.Status == model.TaskStatusFailure {
		switch task.Action {
		case constant.TaskActionVideoGenerationV2:
			taskDto.FailReason = "Video generation failed"
		case constant.TaskActionAudioSpeech:
			taskDto.FailReason = "Audio generation failed"
		default:
			taskDto.FailReason = "Generation task failed"
		}
	}
	switch task.Properties.OriginModelName {
	case constant.AutoDLModelMiniMaxH3:
		taskDto.Platform = "minimax"
		if task.Status == model.TaskStatusSuccess {
			taskDto.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
	case constant.AutoDLModelIndexTTS2:
		taskDto.Platform = "indextts"
		taskDto.ResultURL = ""
	default:
		taskDto.Platform = "custom"
		taskDto.ResultURL = ""
	}
	return taskDto
}

// TaskModel2Dto returns the client-safe task representation.
func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return taskModel2Dto(task, false)
}

// TaskModel2AdminDto retains routing diagnostics for authenticated admin views.
func TaskModel2AdminDto(task *model.Task) *dto.TaskDto {
	return taskModel2Dto(task, true)
}
