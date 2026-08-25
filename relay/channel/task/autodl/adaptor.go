package autodl

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
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey      string
	baseURL     string
	workflowID  string
	request     *dto.MiniMaxVideoGenerationV2Request
	requestBody map[string]any
}

type taskPollError struct {
	message   string
	temporary bool
}

func (e *taskPollError) Error() string {
	return e.message
}

func (e *taskPollError) Temporary() bool {
	return e.temporary
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c.Request.Method != http.MethodPost || c.Request.URL.Path != "/v2/video_generation" {
		return service.TaskErrorWrapperLocal(
			errors.New("AutoDL MiniMax-H3 only supports POST /v2/video_generation"),
			"unsupported_endpoint",
			http.StatusBadRequest,
		)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if storage.Size() > maxRequestBodyBytes {
		return service.TaskErrorWrapperLocal(
			errors.New("request body must not exceed 64 MiB"),
			"request_body_too_large",
			http.StatusRequestEntityTooLarge,
		)
	}

	var request dto.MiniMaxVideoGenerationV2Request
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	workflowID, payload, properties, err := buildWorkflowRequest(&request)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	a.request = &request
	a.workflowID = workflowID
	a.requestBody = payload
	info.Action = constant.TaskActionVideoGenerationV2
	info.Video = properties
	return nil
}

func (a *TaskAdaptor) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	if a.request == nil || a.request.Duration == nil {
		return nil
	}
	return map[string]float64{"seconds": float64(*a.request.Duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	if a.workflowID == "" {
		return "", errors.New("AutoDL workflow was not selected")
	}
	return buildAutoDLURL(a.baseURL, "api", "v1", "comfyui", "comfyui_workflow", a.workflowID)
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(_ *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	if a.requestBody == nil {
		return nil, errors.New("AutoDL request was not validated")
	}
	data, err := common.Marshal(a.requestBody)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// PreflightDispatch proves all deterministic local request construction can
// succeed before billing crosses the provider-call boundary. It performs no
// network I/O.
func (a *TaskAdaptor) PreflightDispatch(c *gin.Context, info *relaycommon.RelayInfo) error {
	endpoint, err := a.BuildRequestURL(info)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build AutoDL request: %w", err)
	}
	if err := a.BuildRequestHeader(c, req, info); err != nil {
		return err
	}
	if _, err := service.GetRelayHttpClientWithProxy(info.ChannelMeta.ChannelSetting.Proxy, false); err != nil {
		return fmt.Errorf("build AutoDL proxy client: %w", err)
	}
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(_ *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	defer resp.Body.Close()
	const maxSubmitResponseBytes = 1 << 20
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSubmitResponseBytes+1))
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusBadGateway)
	}
	if len(responseBody) > maxSubmitResponseBytes {
		return "", nil, service.TaskErrorWrapper(
			errors.New("AutoDL response exceeded the 1 MiB limit"),
			"invalid_upstream_response",
			http.StatusBadGateway,
		)
	}

	var workflowResp workflowResponse
	if err := common.Unmarshal(responseBody, &workflowResp); err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("decode AutoDL response: %w", err),
			"invalid_upstream_response",
			http.StatusBadGateway,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(workflowResp.Code), "Success") {
		return "", nil, service.TaskErrorWrapper(
			errors.New("AutoDL returned an unexpected submission response"),
			"invalid_upstream_response",
			http.StatusBadGateway,
		)
	}
	if strings.TrimSpace(workflowResp.Data.TaskID) == "" {
		return "", nil, service.TaskErrorWrapper(
			errors.New("AutoDL response did not include task_id"),
			"invalid_upstream_response",
			http.StatusBadGateway,
		)
	}

	status := strings.TrimSpace(workflowResp.Data.Status)
	if status == "" {
		status = autoDLStatusQueued
	}
	taskData, err := common.Marshal(map[string]any{
		"code": "Success",
		"data": map[string]any{"status": status},
	})
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "encode_task_data_failed", http.StatusInternalServerError)
	}
	return workflowResp.Data.TaskID, taskData, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	return a.FetchTaskWithContext(context.Background(), baseURL, key, body, proxy)
}

func (a *TaskAdaptor) FetchTaskWithContext(ctx context.Context, baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}

	endpoint, err := buildAutoDLURL(baseURL, "api", "v1", "comfyui", "comfyui_workflow", "result", taskID)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("create proxy HTTP client: %w", err)
	}
	boundedClient := *client
	boundedClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	const pollRequestTimeout = 30 * time.Second
	if boundedClient.Timeout == 0 || boundedClient.Timeout > pollRequestTimeout {
		boundedClient.Timeout = pollRequestTimeout
	}
	return boundedClient.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response workflowResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, &taskPollError{
			message:   fmt.Sprintf("decode AutoDL task result: %v", err),
			temporary: true,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(response.Code), "Success") {
		normalizedCode := strings.ToLower(strings.TrimSpace(response.Code))
		temporary := !strings.Contains(normalizedCode, "unauthor") &&
			!strings.Contains(normalizedCode, "forbidden") &&
			!strings.Contains(normalizedCode, "permission") &&
			!strings.Contains(normalizedCode, "invalid") &&
			!strings.Contains(normalizedCode, "parameter")
		return nil, &taskPollError{
			message:   fmt.Sprintf("AutoDL task query failed (%s)", response.Code),
			temporary: temporary,
		}
	}
	if strings.TrimSpace(response.Data.TaskID) == "" {
		return nil, &taskPollError{message: "AutoDL task query response did not include task_id", temporary: true}
	}

	result := &relaycommon.TaskInfo{TaskID: response.Data.TaskID}
	switch strings.ToUpper(strings.TrimSpace(response.Data.Status)) {
	case autoDLStatusQueued, "PENDING", "SUBMITTED":
		result.Status = model.TaskStatusQueued
		result.Progress = taskcommon.ProgressQueued
	case autoDLStatusRunning, "PROCESSING", "IN_PROGRESS":
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	case autoDLStatusSuccess, autoDLStatusCompleted:
		result.Status = model.TaskStatusSuccess
		result.Progress = taskcommon.ProgressComplete
		for _, output := range response.Data.Results {
			fileType := strings.ToLower(strings.TrimSpace(output.FileType))
			isVideo := strings.EqualFold(output.Type, "video") ||
				strings.EqualFold(output.OutputType, "video") ||
				fileType == "video" || fileType == "mp4" || fileType == ".mp4" ||
				strings.HasPrefix(fileType, "video/")
			if !isVideo || strings.TrimSpace(output.URL) == "" {
				continue
			}
			videoURL, _, err := validateMediaURL(output.URL, mediaKindImage, 0)
			if err != nil || strings.HasPrefix(strings.ToLower(strings.TrimSpace(videoURL)), "data:") {
				return nil, &taskPollError{message: "AutoDL returned an unsafe video URL"}
			}
			result.Url = videoURL
			break
		}
		if result.Url == "" {
			return nil, &taskPollError{message: "AutoDL task succeeded without a video result"}
		}
	case "CANCELLED":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = "AutoDL task cancelled"
	case autoDLStatusFailed, autoDLStatusFailure, "FAIL", "ERROR":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = "AutoDL task failed"
	default:
		return nil, &taskPollError{message: "AutoDL returned an unknown task status", temporary: true}
	}
	return result, nil
}

func (a *TaskAdaptor) SanitizeTaskResult(respBody []byte) []byte {
	var response workflowResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return []byte("{}")
	}
	data := map[string]any{"status": response.Data.Status}
	if len(response.Data.Results) > 0 {
		data["results"] = response.Data.Results
	}
	stored, err := common.Marshal(map[string]any{
		"code": response.Code,
		"data": data,
	})
	if err != nil {
		return []byte("{}")
	}
	return stored
}

func (a *TaskAdaptor) ConvertToMiniMaxVideoV2(originTask *model.Task) ([]byte, error) {
	if originTask == nil {
		return nil, errors.New("task is required")
	}

	createdAt := originTask.CreatedAt
	if createdAt == 0 {
		createdAt = originTask.SubmitTime
	}
	updatedAt := originTask.UpdatedAt
	if updatedAt == 0 {
		updatedAt = createdAt
	}
	status, err := miniMaxTaskStatus(originTask.Status)
	if err != nil {
		return nil, err
	}

	videoTask := dto.MiniMaxVideoTask{
		ID:        originTask.TaskID,
		Model:     originTask.Properties.OriginModelName,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		TaskType:  "generation",
		Modality:  "video",
	}
	if originTask.Status == model.TaskStatusFailure {
		var storedResult workflowResponse
		if common.Unmarshal(originTask.Data, &storedResult) == nil &&
			strings.EqualFold(strings.TrimSpace(storedResult.Data.Status), "CANCELLED") {
			videoTask.Status = "cancelled"
		}
	}
	if videoTask.Model == "" {
		videoTask.Model = "MiniMax-H3"
	}
	if properties := originTask.Properties.Video; properties != nil {
		videoTask.Resolution = properties.Resolution
		videoTask.Duration = properties.Duration
		videoTask.Ratio = properties.Ratio
		if originTask.Status == model.TaskStatusSuccess {
			videoTask.Usage = &dto.MiniMaxVideoTaskUsage{
				TotalSeconds:    properties.Duration,
				OutputSeconds:   properties.Duration,
				InputImageCount: properties.InputImageCount,
			}
		}
	}
	if originTask.Status == model.TaskStatusSuccess {
		resultURL := strings.TrimSpace(originTask.PrivateData.ResultURL)
		validatedURL, _, err := validateMediaURL(resultURL, mediaKindImage, 0)
		if err != nil || strings.HasPrefix(strings.ToLower(validatedURL), "data:") {
			return nil, errors.New("successful AutoDL task has no valid HTTPS video result")
		}
		videoTask.Content = &dto.MiniMaxVideoTaskOutput{URL: validatedURL}
	}
	if originTask.Status == model.TaskStatusFailure {
		if videoTask.Status != "cancelled" {
			message := strings.TrimSpace(originTask.FailReason)
			if message == "" {
				message = "AutoDL task failed"
			}
			videoTask.Error = &dto.MiniMaxVideoTaskError{
				Code:    "generation_failed",
				Message: message,
			}
		}
	}

	return common.Marshal(dto.MiniMaxVideoGenerationV2QueryResponse{Task: videoTask})
}

func buildWorkflowRequest(request *dto.MiniMaxVideoGenerationV2Request) (string, map[string]any, *relaycommon.TaskVideoProperties, error) {
	if request.Model != "MiniMax-H3" {
		return "", nil, nil, errors.New("model must be MiniMax-H3")
	}
	if request.Resolution == nil {
		return "", nil, nil, errors.New("resolution is required")
	}
	if len(*request.Resolution) > 16 {
		return "", nil, nil, errors.New("resolution is invalid")
	}
	if *request.Resolution != "768P" {
		return "", nil, nil, errors.New("AutoDL MiniMax-H3 supports resolution 768P only")
	}
	if request.Duration == nil {
		return "", nil, nil, errors.New("duration is required")
	}
	if *request.Duration < 4 || *request.Duration > 15 {
		return "", nil, nil, errors.New("duration must be an integer between 4 and 15")
	}
	if request.CallbackURL != nil && strings.TrimSpace(*request.CallbackURL) != "" {
		return "", nil, nil, errors.New("callback_url is not supported by the AutoDL workflow backend")
	}
	if request.AIGCWatermark != nil && *request.AIGCWatermark {
		return "", nil, nil, errors.New("aigc_watermark=true is not supported by the AutoDL workflow backend")
	}

	content, err := summarizeContent(request.Content)
	if err != nil {
		return "", nil, nil, err
	}

	ratio := ""
	if request.Ratio != nil {
		if len(*request.Ratio) > 16 {
			return "", nil, nil, errors.New("ratio is invalid")
		}
		ratio = strings.TrimSpace(*request.Ratio)
	}
	if ratio == "" || ratio == "adaptive" {
		return "", nil, nil, errors.New("AutoDL workflows require an explicit non-adaptive ratio because they cannot infer the input media aspect ratio")
	}
	workflowID := ""
	supportsSquare := false
	maxPromptLength := 0
	payload := map[string]any{
		"prompt":   content.Prompt,
		"duration": *request.Duration,
	}

	switch {
	case content.FirstFrame != "" || content.LastFrame != "":
		return "", nil, nil, errors.New("first_frame and last_frame inputs are not supported because the AutoDL workflow cannot preserve MiniMax V2 adaptive aspect-ratio semantics")
	case len(content.ReferenceAudios) > 0:
		if len(content.ReferenceImages) == 0 {
			return "", nil, nil, errors.New("reference_audio requires at least one reference_image for the AutoDL workflow")
		}
		maxPromptLength = 10_000
		if *request.Duration > 10 {
			workflowID = workflowReferenceImageAudio15s
		} else {
			workflowID = workflowReferenceImageAudio
		}
		for index, mediaURL := range content.ReferenceImages {
			payload["ref_image_"+strconv.Itoa(index)] = mediaURL
		}
		for index, mediaURL := range content.ReferenceAudios {
			payload["ref_audio_"+strconv.Itoa(index)] = mediaURL
		}
	case len(content.ReferenceImages) > 0:
		supportsSquare = true
		maxPromptLength = 500_000
		if *request.Duration > 10 {
			workflowID = workflowReferenceImages15s
		} else {
			workflowID = workflowReferenceImages
		}
		for index, mediaURL := range content.ReferenceImages {
			payload["ref_image_"+strconv.Itoa(index)] = mediaURL
		}
	default:
		workflowID = workflowTextToVideo
		supportsSquare = true
		maxPromptLength = 200_000
	}
	if utf8.RuneCountInString(content.Prompt) > maxPromptLength {
		return "", nil, nil, fmt.Errorf("combined text content exceeds the selected AutoDL workflow limit of %d characters", maxPromptLength)
	}

	resolution, actualRatio, err := mapResolution(ratio, supportsSquare)
	if err != nil {
		return "", nil, nil, err
	}
	payload["resolution"] = resolution
	properties := &relaycommon.TaskVideoProperties{
		Resolution:      "768P",
		Duration:        *request.Duration,
		Ratio:           actualRatio,
		InputImageCount: len(content.ReferenceImages),
	}
	return workflowID, payload, properties, nil
}

func summarizeContent(items []dto.MiniMaxVideoContentItem) (contentSummary, error) {
	if len(items) == 0 {
		return contentSummary{}, errors.New("content is required")
	}

	var summary contentSummary
	var prompts []string
	var unassignedImages []string
	totalDataBytes := 0
	for _, item := range items {
		if len(item.Type) > 32 || len(item.Role) > 32 {
			return contentSummary{}, errors.New("content item type or role is invalid")
		}
		switch item.Type {
		case "text":
			text := strings.TrimSpace(item.Text)
			if text != "" {
				if utf8.RuneCountInString(text) > 7000 {
					return contentSummary{}, errors.New("each text item must not exceed 7000 characters")
				}
				prompts = append(prompts, text)
			}
		case "image_url":
			if item.ImageURL == nil {
				return contentSummary{}, errors.New("image_url.url is required")
			}
			mediaURL, updatedDataBytes, err := validateMediaURL(item.ImageURL.URL, mediaKindImage, totalDataBytes)
			if err != nil {
				return contentSummary{}, fmt.Errorf("image_url: %w", err)
			}
			totalDataBytes = updatedDataBytes
			switch item.Role {
			case "":
				unassignedImages = append(unassignedImages, mediaURL)
			case miniMaxRoleFirstFrame:
				if summary.FirstFrame != "" {
					return contentSummary{}, errors.New("only one first_frame is allowed")
				}
				summary.FirstFrame = mediaURL
			case miniMaxRoleLastFrame:
				if summary.LastFrame != "" {
					return contentSummary{}, errors.New("only one last_frame is allowed")
				}
				summary.LastFrame = mediaURL
			case miniMaxRoleReferenceImage:
				summary.ReferenceImages = append(summary.ReferenceImages, mediaURL)
			default:
				return contentSummary{}, errors.New("image_url role is invalid")
			}
		case "audio_url":
			if item.AudioURL == nil {
				return contentSummary{}, errors.New("audio_url.url is required")
			}
			if item.Role != miniMaxRoleReferenceAudio {
				return contentSummary{}, errors.New("audio_url role must be reference_audio")
			}
			mediaURL, updatedDataBytes, err := validateMediaURL(item.AudioURL.URL, mediaKindAudio, totalDataBytes)
			if err != nil {
				return contentSummary{}, fmt.Errorf("audio_url: %w", err)
			}
			totalDataBytes = updatedDataBytes
			summary.ReferenceAudios = append(summary.ReferenceAudios, mediaURL)
		case "video_url":
			if item.Role != miniMaxRoleReferenceVideo {
				return contentSummary{}, errors.New("video_url role must be reference_video")
			}
			return contentSummary{}, errors.New("reference_video is not supported by the AutoDL workflow backend")
		default:
			return contentSummary{}, errors.New("content item type is unsupported")
		}
	}

	if len(prompts) == 0 {
		return contentSummary{}, errors.New("content must include a non-empty text item")
	}
	if len(unassignedImages) > 1 {
		return contentSummary{}, errors.New("multiple image_url items must declare a role")
	}
	if len(unassignedImages) == 1 {
		if summary.FirstFrame != "" || summary.LastFrame != "" || len(summary.ReferenceImages) > 0 {
			return contentSummary{}, errors.New("an image without role cannot be combined with other images")
		}
		summary.FirstFrame = unassignedImages[0]
	}
	if len(summary.ReferenceImages) > 9 {
		return contentSummary{}, errors.New("at most 9 reference images are supported")
	}
	if len(summary.ReferenceAudios) > 3 {
		return contentSummary{}, errors.New("at most 3 reference audio files are supported")
	}
	if (summary.FirstFrame != "" || summary.LastFrame != "") && (len(summary.ReferenceImages) > 0 || len(summary.ReferenceAudios) > 0) {
		return contentSummary{}, errors.New("first/last frames cannot be combined with reference media")
	}

	summary.Prompt = strings.Join(prompts, "\n")
	return summary, nil
}

func mapResolution(ratio string, supportsSquare bool) (string, string, error) {
	switch ratio {
	case "9:16":
		return "768p竖", "9:16", nil
	case "16:9":
		return "768p横", "16:9", nil
	case "1:1":
		if supportsSquare {
			return "768p(1:1)", "1:1", nil
		}
		return "", "", errors.New("ratio 1:1 is not supported for this AutoDL workflow")
	case "21:9", "4:3", "3:4":
		return "", "", errors.New("ratio is not available in the AutoDL 768P workflows")
	default:
		return "", "", errors.New("ratio is invalid")
	}
}

func miniMaxTaskStatus(status model.TaskStatus) (string, error) {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded", nil
	case model.TaskStatusFailure:
		return "failed", nil
	case model.TaskStatusInProgress:
		return "running", nil
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusReserving, model.TaskStatusCheckpointPending:
		return "queued", nil
	}
	return "", fmt.Errorf("unsupported AutoDL task status %q", status)
}

var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)

var _ channel.MiniMaxVideoV2Converter = (*TaskAdaptor)(nil)
