package depthmedia

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	ActionDepth = "depth"
	ActionMedia = "media"

	maxDepthVideoBillingSeconds = 10 * 60

	ModelDepthVideo        = "depth-anything-v2-small-video"
	ModelBackgroundFast    = "background-remove-fast"
	ModelBackgroundQuality = "background-remove-quality"
	ModelBackgroundMatting = "background-remove-matting"
	ModelUpscaleFast2X     = "image-upscale-fast-2x"
	ModelUpscaleFast4X     = "image-upscale-fast-4x"
	ModelUpscaleFidelity4X = "image-upscale-fidelity-4x"
	ModelUpscaleSharp4X    = "image-upscale-sharp-4x"
)

var supportedModels = []string{
	ModelDepthVideo,
	ModelBackgroundFast,
	ModelBackgroundQuality,
	ModelBackgroundMatting,
	ModelUpscaleFast2X,
	ModelUpscaleFast4X,
	ModelUpscaleFidelity4X,
	ModelUpscaleSharp4X,
}

type requestPayload struct {
	SourceURL string `json:"source_url"`
	Operation string `json:"operation,omitempty"`
	Quality   string `json:"quality,omitempty"`
	Scale     int    `json:"scale,omitempty"`
	Format    string `json:"format,omitempty"`
}

type responsePayload struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Progress  int     `json:"progress"`
	ResultURL string  `json:"result_url,omitempty"`
	Error     string  `json:"error,omitempty"`
	FPS       float64 `json:"fps,omitempty"`
	Frames    int     `json:"frames,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL string
	apiKey  string
}

func ResolveModel(operation, quality string, scale int) (string, error) {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "remove_background":
		switch strings.ToLower(strings.TrimSpace(quality)) {
		case "fast":
			return ModelBackgroundFast, nil
		case "quality":
			return ModelBackgroundQuality, nil
		case "matting":
			return ModelBackgroundMatting, nil
		}
	case "upscale":
		switch strings.ToLower(strings.TrimSpace(quality)) {
		case "fast":
			if scale == 2 {
				return ModelUpscaleFast2X, nil
			}
			if scale == 4 {
				return ModelUpscaleFast4X, nil
			}
		case "fidelity":
			if scale == 4 {
				return ModelUpscaleFidelity4X, nil
			}
		case "sharp":
			if scale == 4 {
				return ModelUpscaleSharp4X, nil
			}
		}
	}
	return "", fmt.Errorf("unsupported media profile: operation=%q quality=%q scale=%d", operation, quality, scale)
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Image) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("source_url is required"), "invalid_request", http.StatusBadRequest)
	}
	sourceURL, err := url.ParseRequestURI(request.Image)
	if err != nil || (sourceURL.Scheme != "http" && sourceURL.Scheme != "https") || sourceURL.Host == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("source_url must be an http or https URL"), "invalid_request", http.StatusBadRequest)
	}
	if request.WebhookURL != "" {
		if err := service.ValidateJSONWebhookURL(request.WebhookURL); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_webhook_url", http.StatusBadRequest)
		}
	}
	action := ActionMedia
	if request.Model == ModelDepthVideo {
		action = ActionDepth
	} else {
		var metadata requestPayload
		if err := request.UnmarshalMetadata(&metadata); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		resolved, err := ResolveModel(metadata.Operation, metadata.Quality, metadata.Scale)
		if err != nil || resolved != request.Model {
			if err == nil {
				err = fmt.Errorf("model does not match media profile")
			}
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}
	info.Action = action
	c.Set("action", action)
	c.Set("task_request", request)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.Action {
	case ActionDepth:
		return a.baseURL + "/v1/depth/jobs", nil
	case ActionMedia:
		return a.baseURL + "/v1/media/jobs", nil
	default:
		return "", fmt.Errorf("unsupported depth media action: %s", info.Action)
	}
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, _ *relaycommon.RelayInfo) error {
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("task request not found")
	}
	request, ok := value.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid task request")
	}
	payload := requestPayload{SourceURL: request.Image}
	if info.Action == ActionMedia {
		if err := request.UnmarshalMetadata(&payload); err != nil {
			return nil, err
		}
		payload.SourceURL = request.Image
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, body)
}

func (a *TaskAdaptor) EstimateBilling(_ *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info.OriginModelName != ModelDepthVideo {
		return nil
	}
	return map[string]float64{"seconds": maxDepthVideoBillingSeconds}
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil || taskResult.Status != model.TaskStatusSuccess ||
		task.Action != ActionDepth {
		return 0
	}
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.OriginModelName != ModelDepthVideo ||
		billing.ModelPrice <= 0 || billing.GroupRatio <= 0 {
		return 0
	}
	var response responsePayload
	if err := common.Unmarshal(task.Data, &response); err != nil ||
		response.FPS <= 0 || response.Frames <= 0 {
		return 0
	}
	seconds := math.Ceil(float64(response.Frames) / response.FPS)
	seconds = min(seconds, maxDepthVideoBillingSeconds)
	quota, clamp := common.QuotaFromFloatChecked(
		billing.ModelPrice * common.QuotaPerUnit * billing.GroupRatio * seconds,
	)
	task.PrivateData.FinalQuotaClamp = clamp
	if quota <= 0 {
		return 0
	}
	return quota
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	var upstream responsePayload
	if err := common.Unmarshal(body, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusBadGateway)
	}
	if upstream.ID == "" {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("upstream task id is missing"), "invalid_upstream_response", http.StatusBadGateway)
	}
	c.JSON(http.StatusAccepted, gin.H{
		"id":         info.PublicTaskID,
		"task_id":    info.PublicTaskID,
		"status":     upstream.Status,
		"progress":   upstream.Progress,
		"created_at": time.Now().Unix(),
	})
	return upstream.ID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	action, ok := body["action"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid action")
	}
	path := "/v1/media/jobs/"
	if action == ActionDepth {
		path = "/v1/depth/jobs/"
	} else if action != ActionMedia {
		return nil, fmt.Errorf("unsupported depth media action: %s", action)
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+path+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response responsePayload
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	info := &relaycommon.TaskInfo{
		TaskID:   response.ID,
		Progress: strconv.Itoa(response.Progress) + "%",
		Reason:   response.Error,
		Url:      response.ResultURL,
	}
	switch strings.ToLower(response.Status) {
	case "queued":
		info.Status = model.TaskStatusQueued
	case "processing":
		info.Status = model.TaskStatusInProgress
	case "completed":
		info.Status = model.TaskStatusSuccess
		info.Progress = "100%"
	case "failed":
		info.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %s", response.Status)
	}
	return info, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), supportedModels...)
}

func (a *TaskAdaptor) GetChannelName() string {
	return "depthmedia"
}
