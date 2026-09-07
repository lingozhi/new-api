package sora

import (
	"fmt"
	"math"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Unified Wan requests retain one public model while selecting the provider workflow.
func validateUnifiedWanVideoRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" || c.Request.URL.Path != "/v1/videos" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("wan3.0 requires JSON POST /v1/videos"), "invalid_request", http.StatusBadRequest)
	}
	var request wanVideoRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var controls struct {
		Mode         *string `json:"mode,omitempty"`
		Speed        *string `json:"speed,omitempty"`
		FirstFrame   *string `json:"first_frame,omitempty"`
		LastFrame    *string `json:"last_frame,omitempty"`
		PromptExtend *bool   `json:"prompt_extend,omitempty"`
	}
	if err := common.UnmarshalBodyReusable(c, &controls); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	allowed := map[string]bool{"model": true, "prompt": true, "seconds": true, "duration": true, "size": true, "resolution": true, "aspect_ratio": true, "ratio": true, "n": true, "prompt_extend": true, "mode": true, "speed": true, "first_frame": true, "last_frame": true, "reference_images": true, "reference_videos": true, "reference_audios": true}
	for key, value := range body {
		if !allowed[key] || value == nil {
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported or null field %s; omit unused fields", key), "invalid_request", http.StatusBadRequest)
		}
	}
	mode := "auto"
	if controls.Mode != nil {
		mode = *controls.Mode
	}
	hasFrames := controls.FirstFrame != nil || controls.LastFrame != nil
	if mode == "auto" {
		mode = "general"
		if hasFrames {
			mode = "frames"
		}
	}
	if mode != "general" && mode != "reference" && mode != "frames" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("mode must be auto, general, reference, or frames"), "invalid_request", http.StatusBadRequest)
	}
	speed := "standard"
	if mode == "reference" {
		speed = "fast"
	}
	if controls.Speed != nil {
		speed = *controls.Speed
	}
	if speed != "standard" && speed != "fast" || mode == "reference" && speed != "fast" || mode == "frames" && speed != "standard" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("general supports standard/fast; reference supports fast; frames supports standard"), "invalid_request", http.StatusBadRequest)
	}
	target := "wan3.0-video"
	if speed == "fast" {
		target = "wan3.0-video-prime"
	}
	if mode == "reference" {
		target = "wan3.0-prime-r2v"
	}
	if mode == "frames" {
		target = "wan3.0-i2v"
	}
	if mode != "frames" && hasFrames {
		return service.TaskErrorWrapperLocal(fmt.Errorf("first_frame/last_frame require frames or auto mode"), "invalid_media", http.StatusBadRequest)
	}
	if mode == "frames" {
		if controls.FirstFrame == nil || controls.LastFrame == nil || strings.TrimSpace(*controls.FirstFrame) == "" || strings.TrimSpace(*controls.LastFrame) == "" || len(request.ReferenceImages)+len(request.ReferenceVideos)+len(request.ReferenceAudios) > 0 {
			return service.TaskErrorWrapperLocal(fmt.Errorf("frames mode requires first_frame and last_frame and no reference lists"), "invalid_media", http.StatusBadRequest)
		}
		request.Media = []wanMedia{{Type: "first_frame", URL: *controls.FirstFrame}, {Type: "last_frame", URL: *controls.LastFrame}}
		body["media"] = request.Media
	}
	for _, field := range []string{"reference_images", "reference_videos", "reference_audios"} {
		if entries, ok := body[field].([]any); ok {
			for _, entry := range entries {
				media, ok := entry.(map[string]any)
				if !ok {
					return service.TaskErrorWrapperLocal(fmt.Errorf("%s entries must be objects", field), "invalid_media", http.StatusBadRequest)
				}
				for key := range media {
					if key != "url" && !(field == "reference_images" && key == "role") && !(field == "reference_videos" && key == "duration") {
						return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported %s field %s", field, key), "invalid_media", http.StatusBadRequest)
					}
				}
				if role, exists := media["role"]; exists {
					if role != "reference_image" && (mode != "general" || role != "first_frame" && role != "last_frame") {
						return service.TaskErrorWrapperLocal(fmt.Errorf("invalid reference image role for selected mode"), "invalid_media", http.StatusBadRequest)
					}
				}
				if raw, exists := media["duration"]; exists {
					duration, ok := raw.(float64)
					if !ok || duration <= 0 || duration > relaycommon.MaxTaskDurationSeconds || math.IsNaN(duration) || math.IsInf(duration, 0) {
						return service.TaskErrorWrapperLocal(fmt.Errorf("invalid reference video duration"), "invalid_media", http.StatusBadRequest)
					}
					if mode == "reference" {
						return service.TaskErrorWrapperLocal(fmt.Errorf("reference video duration is supported only in general mode"), "invalid_media", http.StatusBadRequest)
					}
				}
			}
		}
	}
	if mode == "reference" {
		for _, image := range request.ReferenceImages {
			request.Media = append(request.Media, wanMedia{Type: "reference_image", URL: image.URL})
		}
		for _, video := range request.ReferenceVideos {
			request.Media = append(request.Media, wanMedia{Type: "reference_video", URL: video.URL})
		}
		for _, audio := range request.ReferenceAudios {
			request.Media = append(request.Media, wanMedia{Type: "audio", URL: audio.URL})
		}
		request.ReferenceImages, request.ReferenceVideos, request.ReferenceAudios = nil, nil, nil
		body["media"] = request.Media
	}
	ratio := request.AspectRatio
	if ratio == "" {
		ratio = request.Ratio
	}
	if ratio == "" {
		ratio = "16:9"
	}
	if ratio != "16:9" && ratio != "9:16" && ratio != "1:1" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("aspect_ratio must be 16:9, 9:16, or 1:1"), "invalid_request", http.StatusBadRequest)
	}
	request.AspectRatio = ratio
	request.Model = "wan3.0"
	if err := validateWanVideoFields(c, info, request, target); err != nil {
		return err
	}
	for _, field := range []string{"mode", "speed", "first_frame", "last_frame"} {
		delete(body, field)
	}
	if mode != "general" {
		for _, field := range []string{"reference_images", "reference_videos", "reference_audios"} {
			delete(body, field)
		}
	}
	body["aspect_ratio"], body["ratio"] = ratio, ratio
	c.Set("wan_unified_body", body)
	c.Set("wan_unified_model", target)
	if info.TaskRelayInfo != nil && info.TaskRelayInfo.Video != nil {
		info.TaskRelayInfo.Video.Provider = "wan-unified"
	}
	return nil
}
