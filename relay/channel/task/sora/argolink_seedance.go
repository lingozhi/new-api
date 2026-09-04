package sora

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	argolinkSeedance25ContextKey        = "argolink_seedance_2_5_request"
	argolinkSeedance25DefaultDuration   = 5
	argolinkSeedance25DefaultResolution = "720p"
	argolinkSeedance25BasePrice         = 0.17
)

var argolinkSeedance25ResolutionPrices = map[string]float64{
	"480p":  0.077,
	"720p":  0.17,
	"1080p": 0.43,
}

type argolinkSeedance25Request struct {
	Model           string             `json:"model"`
	Prompt          string             `json:"prompt,omitempty"`
	Duration        *int               `json:"duration,omitempty"`
	Resolution      string             `json:"resolution,omitempty"`
	AspectRatio     string             `json:"aspect_ratio,omitempty"`
	N               *int               `json:"n,omitempty"`
	StartImage      *argolinkMediaRef  `json:"start_image,omitempty"`
	EndImage        *argolinkMediaRef  `json:"end_image,omitempty"`
	ReferenceImages []argolinkMediaRef `json:"reference_images,omitempty"`
	ReferenceVideos []argolinkMediaRef `json:"reference_videos,omitempty"`
	ReferenceAudios []argolinkMediaRef `json:"reference_audios,omitempty"`
}

type argolinkMediaRef struct {
	URL string `json:"url,omitempty"`
}

type argolinkPublicVideo struct {
	Duration int    `json:"duration,omitempty"`
	URL      string `json:"url,omitempty"`
}

type argolinkPublicError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
}

type argolinkPublicResponse struct {
	Model     string               `json:"model"`
	RequestID string               `json:"request_id"`
	Status    string               `json:"status"`
	Video     *argolinkPublicVideo `json:"video,omitempty"`
	Error     *argolinkPublicError `json:"error,omitempty"`
}

func isArgolinkSeedanceModel(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), constant.ArgolinkSeedance25Model) || strings.EqualFold(strings.TrimSpace(modelName), constant.ArgolinkSeedance20Model) || strings.EqualFold(strings.TrimSpace(modelName), constant.ArgolinkSeedance20FastModel)
}

func isArgolinkSeedanceRequest(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c != nil && c.Request != nil && c.Request.URL.Path == "/v1/videos/generations" {
		return true
	}
	return info != nil && isArgolinkSeedanceModel(info.OriginModelName)
}

func validateArgolinkSeedanceRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request argolinkSeedance25Request
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if !isArgolinkSeedanceModel(request.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model must be seedance-2.0, seedance-2.0-fast, or seedance-2.5"),
			"invalid_model",
			http.StatusBadRequest,
		)
	}
	hasMedia := request.StartImage != nil || request.EndImage != nil ||
		len(request.ReferenceImages) > 0 || len(request.ReferenceVideos) > 0 || len(request.ReferenceAudios) > 0
	if strings.TrimSpace(request.Prompt) == "" && !hasMedia {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required for text-to-video"), "invalid_request", http.StatusBadRequest)
	}

	duration := argolinkSeedance25DefaultDuration
	if request.Duration != nil {
		duration = *request.Duration
	}
	maxDuration := 30
	if !strings.EqualFold(strings.TrimSpace(request.Model), constant.ArgolinkSeedance25Model) {
		maxDuration = 15
	}
	if duration < 4 || duration > maxDuration {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 4 and %d seconds", maxDuration), "invalid_duration", http.StatusBadRequest)
	}
	if request.N != nil && *request.N != 1 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("n must be 1"), "invalid_n", http.StatusBadRequest)
	}

	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	if resolution == "" {
		resolution = argolinkSeedance25DefaultResolution
	}
	if _, ok := argolinkSeedance25ResolutionPrices[resolution]; !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("resolution must be 480p, 720p, or 1080p"), "invalid_resolution", http.StatusBadRequest)
	}
	if maxDuration == 15 && (!hasMedia || strings.EqualFold(strings.TrimSpace(request.Model), constant.ArgolinkSeedance20FastModel)) && resolution == "1080p" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("this model and input mode support 480p or 720p"), "invalid_resolution", http.StatusBadRequest)
	}
	request.Resolution = resolution
	c.Set(argolinkSeedance25ContextKey, request)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    request.Model,
		Prompt:   request.Prompt,
		Duration: duration,
		Size:     resolution,
	})
	info.Action = constant.TaskActionTextGenerate
	if hasMedia {
		info.Action = constant.TaskActionGenerate
	}
	if info.TaskRelayInfo != nil {
		info.TaskRelayInfo.Video = &relaycommon.TaskVideoProperties{
			Resolution:      resolution,
			Duration:        duration,
			Ratio:           request.AspectRatio,
			InputImageCount: len(request.ReferenceImages),
		}
		if request.StartImage != nil {
			info.TaskRelayInfo.Video.InputImageCount++
		}
		if request.EndImage != nil {
			info.TaskRelayInfo.Video.InputImageCount++
		}
	}
	return nil
}

func estimateArgolinkSeedanceBilling(c *gin.Context) map[string]float64 {
	value, ok := c.Get(argolinkSeedance25ContextKey)
	if !ok {
		return nil
	}
	request, ok := value.(argolinkSeedance25Request)
	if !ok {
		return nil
	}
	duration := argolinkSeedance25DefaultDuration
	if request.Duration != nil {
		duration = *request.Duration
	}
	price := argolinkSeedance25ResolutionPrices[request.Resolution]
	basePrice, videoInputRatio := argolinkSeedance25BasePrice, 1.6
	if !strings.EqualFold(strings.TrimSpace(request.Model), constant.ArgolinkSeedance25Model) {
		basePrice, videoInputRatio = 0.11, 2
		switch request.Resolution {
		case "480p":
			price = 0.05
		case "720p":
			price = 0.11
		case "1080p":
			price = 0.28
		}
	}
	if strings.EqualFold(strings.TrimSpace(request.Model), constant.ArgolinkSeedance20FastModel) {
		basePrice = 0.091
		if request.Resolution == "480p" {
			price = 0.04
		} else {
			price = 0.091
		}
	}
	ratios := map[string]float64{
		"seconds":    float64(duration),
		"resolution": price / basePrice,
	}
	if len(request.ReferenceVideos) > 0 {
		ratios["video_input"] = videoInputRatio
	}
	return ratios
}

func convertArgolinkSeedanceTask(task *model.Task) ([]byte, error) {
	var upstream responseTask
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &upstream)
	}
	status := "pending"
	response := argolinkPublicResponse{
		Model:     task.Properties.OriginModelName,
		RequestID: task.TaskID,
		Status:    status,
	}
	switch task.Status {
	case model.TaskStatusSuccess:
		response.Status = "done"
		duration := 0
		if task.Properties.Video != nil {
			duration = task.Properties.Video.Duration
		}
		if upstream.Video != nil && upstream.Video.Duration > 0 {
			duration = upstream.Video.Duration
		}
		response.Video = &argolinkPublicVideo{
			Duration: duration,
			URL:      fmt.Sprintf("/v1/videos/%s/content", task.TaskID),
		}
	case model.TaskStatusFailure:
		response.Status = "failed"
		message := task.FailReason
		if message == "" && upstream.Error != nil {
			message = upstream.Error.Message
		}
		if message == "" {
			message = "video generation failed"
		}
		response.Error = &argolinkPublicError{Message: message}
	case model.TaskStatusInProgress:
		response.Status = "pending"
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		response.Status = "pending"
	}
	return common.Marshal(response)
}

// AdjustBillingOnComplete reconciles against delivered seconds using the saved price.
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, result *relaycommon.TaskInfo) int {
	if task == nil || result == nil || result.Status != model.TaskStatusSuccess || !isArgolinkSeedanceModel(task.Properties.OriginModelName) {
		return 0
	}
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.ModelPrice <= 0 || billing.GroupRatio <= 0 {
		return 0
	}
	var response responseTask
	if err := common.Unmarshal(task.Data, &response); err != nil || response.Video == nil || response.Video.Duration <= 0 {
		return 0
	}
	seconds := min(response.Video.Duration, relaycommon.MaxTaskDurationSeconds)
	price := &types.PriceData{}
	for name, ratio := range billing.OtherRatios {
		if name != "seconds" {
			price.AddOtherRatio(name, ratio)
		}
	}
	quota, clamp := common.QuotaFromFloatChecked(price.ApplyOtherRatiosToFloat(billing.ModelPrice * common.QuotaPerUnit * billing.GroupRatio * float64(seconds)))
	task.PrivateData.FinalQuotaClamp = clamp
	return quota
}
