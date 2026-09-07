package sora

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type wanMedia struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url"`
}

type wanVideoRequest struct {
	Model           string     `json:"model"`
	Prompt          string     `json:"prompt"`
	Seconds         *string    `json:"seconds,omitempty"`
	Duration        *int       `json:"duration,omitempty"`
	Size            string     `json:"size,omitempty"`
	Resolution      string     `json:"resolution,omitempty"`
	AspectRatio     string     `json:"aspect_ratio,omitempty"`
	Ratio           string     `json:"ratio,omitempty"`
	N               *int       `json:"n,omitempty"`
	Media           []wanMedia `json:"media,omitempty"`
	ReferenceImages []wanMedia `json:"reference_images,omitempty"`
	ReferenceVideos []wanMedia `json:"reference_videos,omitempty"`
	ReferenceAudios []wanMedia `json:"reference_audios,omitempty"`
}

// validateWanVideoRequest normalizes the aliases used for both billing and relay.
// Defaults must also be sent upstream so omitted parameters cannot undercharge.
func validateWanVideoRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request wanVideoRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	duration := 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	if request.Seconds != nil {
		seconds, err := strconv.Atoi(*request.Seconds)
		if err != nil || (request.Duration != nil && seconds != duration) {
			return service.TaskErrorWrapperLocal(fmt.Errorf("seconds must be an integer and agree with duration"), "invalid_duration", http.StatusBadRequest)
		}
		duration = seconds
	}
	if duration < 2 || duration > 30 || duration > relaycommon.MaxTaskDurationSeconds {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 2 and 30 seconds"), "invalid_duration", http.StatusBadRequest)
	}
	if request.N != nil && *request.N != 1 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("n must be 1"), "invalid_n", http.StatusBadRequest)
	}
	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	size := strings.ToLower(strings.TrimSpace(request.Size))
	if resolution != "" && size != "" && resolution != size {
		return service.TaskErrorWrapperLocal(fmt.Errorf("size and resolution must agree"), "invalid_resolution", http.StatusBadRequest)
	}
	if resolution == "" {
		resolution = size
	}
	if resolution == "" {
		resolution = "720p"
	}
	if _, ok := common.WanVideoResolutionRatios(info.OriginModelName)[resolution]; !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("resolution must be 480p, 720p, or 1080p"), "invalid_resolution", http.StatusBadRequest)
	}
	ratio := strings.TrimSpace(request.AspectRatio)
	if request.Ratio != "" && ratio != "" && request.Ratio != ratio {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio and aspect_ratio must agree"), "invalid_request", http.StatusBadRequest)
	}
	if ratio == "" {
		ratio = request.Ratio
	}
	imageCount := len(request.ReferenceImages)
	hasReferences := imageCount+len(request.ReferenceVideos)+len(request.ReferenceAudios) > 0
	if len(request.ReferenceImages) > 10 || len(request.ReferenceVideos) > 5 || len(request.ReferenceAudios) > 5 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference limits are 10 images, 5 videos, and 5 audios"), "invalid_media", http.StatusBadRequest)
	}
	mediaModel := info.OriginModelName == "wan3.0-prime-r2v" || info.OriginModelName == "wan3.0-i2v"
	if mediaModel && (hasReferences || len(request.Media) == 0) || !mediaModel && len(request.Media) > 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("media and reference_* fields must match the Wan model"), "invalid_media", http.StatusBadRequest)
	}
	counts := make(map[string]int)
	for _, media := range request.Media {
		counts[media.Type]++
		validType := media.Type == "reference_image" || media.Type == "reference_video" || media.Type == "audio"
		if info.OriginModelName == "wan3.0-i2v" {
			validType = media.Type == "first_frame" || media.Type == "last_frame"
		}
		if !validType {
			return service.TaskErrorWrapperLocal(fmt.Errorf("invalid media type for Wan model"), "invalid_media", http.StatusBadRequest)
		}
	}
	if info.OriginModelName == "wan3.0-i2v" && (counts["first_frame"] != 1 || counts["last_frame"] != 1) ||
		counts["reference_image"] > 10 || counts["reference_video"] > 5 || counts["audio"] > 5 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid Wan media counts"), "invalid_media", http.StatusBadRequest)
	}
	for _, entries := range [][]wanMedia{request.Media, request.ReferenceImages, request.ReferenceVideos, request.ReferenceAudios} {
		for _, media := range entries {
			parsed, err := url.Parse(media.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
				return service.TaskErrorWrapperLocal(fmt.Errorf("media URL must use HTTPS"), "invalid_media", http.StatusBadRequest)
			}
		}
	}
	imageCount += counts["reference_image"] + counts["first_frame"] + counts["last_frame"]
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: request.Model, Prompt: request.Prompt, Duration: duration,
		Seconds: strconv.Itoa(duration), Size: resolution,
	})
	info.Action = constant.TaskActionTextGenerate
	if hasReferences || len(request.Media) > 0 {
		info.Action = constant.TaskActionGenerate
	}
	if info.TaskRelayInfo != nil {
		info.TaskRelayInfo.Video = &relaycommon.TaskVideoProperties{
			Resolution: resolution, Duration: duration, Ratio: ratio, InputImageCount: imageCount,
		}
	}
	return nil
}
