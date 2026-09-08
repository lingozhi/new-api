package sora

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Lxmone and Argolink share a public model name, but not their wire or billing contract.
func isLxmoneSeedanceRequest(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	return common.IsLxmoneSeedance(info.ChannelBaseUrl, info.OriginModelName)
}

type lxmoneSeedanceRequest struct {
	Prompt          string            `json:"prompt"`
	Seconds         json.RawMessage   `json:"seconds"`
	Duration        *int              `json:"duration,omitempty"`
	Size            string            `json:"size"`
	Resolution      string            `json:"resolution"`
	AspectRatio     string            `json:"aspect_ratio"`
	Ratio           string            `json:"ratio"`
	InputReference  json.RawMessage   `json:"input_reference"`
	Image           json.RawMessage   `json:"image"`
	ImageEnd        json.RawMessage   `json:"image_end"`
	EndImageURL     json.RawMessage   `json:"end_image_url"`
	ReferenceImages []json.RawMessage `json:"reference_images"`
	ReferenceVideos []json.RawMessage `json:"reference_videos"`
	ReferenceAudios []json.RawMessage `json:"reference_audios"`
	SoundEffects    *bool             `json:"sound_effects,omitempty"`
	NoMusic         *bool             `json:"no_music,omitempty"`
}

// Accept the website's older {url} inputs, but send the provider's URL strings.
func lxmoneSeedanceMediaURL(raw json.RawMessage) (string, error) {
	var value string
	if common.GetJsonType(raw) == "object" {
		var item map[string]json.RawMessage
		if err := common.Unmarshal(raw, &item); err != nil {
			return "", err
		}
		if len(item) != 1 || item["url"] == nil {
			return "", fmt.Errorf("media objects must contain only url")
		}
		raw = item["url"]
	}
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("media must be a URL string or {url} object")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("media must be a public HTTP(S) URL without credentials")
	}
	return value, nil
}

func validateLxmoneSeedanceRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request lxmoneSeedanceRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var fields map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &fields); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	for _, key := range []string{"stream", "n", "response_format"} {
		if _, exists := fields[key]; exists {
			return service.TaskErrorWrapperLocal(fmt.Errorf("%s is not supported by this provider", key), "invalid_request", http.StatusBadRequest)
		}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	duration := 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	if len(request.Seconds) > 0 {
		var seconds int
		var err error
		if request.Seconds[0] == '"' {
			var value string
			err = common.Unmarshal(request.Seconds, &value)
			if err == nil {
				seconds, err = strconv.Atoi(value)
			}
		} else {
			err = common.Unmarshal(request.Seconds, &seconds)
		}
		if err != nil || request.Duration != nil && seconds != duration {
			return service.TaskErrorWrapperLocal(fmt.Errorf("seconds must be an integer and agree with duration"), "invalid_duration", http.StatusBadRequest)
		}
		duration = seconds
	}
	upstreamModel := common.LxmoneSeedanceModel(info.OriginModelName)
	if request.SoundEffects != nil && request.NoMusic != nil && *request.SoundEffects == *request.NoMusic {
		return service.TaskErrorWrapperLocal(fmt.Errorf("sound_effects and no_music must be opposite when both are supplied"), "invalid_request", http.StatusBadRequest)
	}
	for _, field := range []string{"sound_effects", "no_music", "input_reference", "image", "image_end", "end_image_url", "reference_images", "reference_videos", "reference_audios"} {
		if raw, exists := fields[field]; exists && strings.TrimSpace(string(raw)) == "null" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("omit unused %s instead of null", field), "invalid_request", http.StatusBadRequest)
		}
	}
	normalizedMedia := make(map[string]any)
	imageLimit, otherLimit := 9, 3
	if upstreamModel == "seedance-2.5-pro" {
		imageLimit, otherLimit = 30, 10
	}
	for _, group := range []struct {
		name  string
		items []json.RawMessage
		limit int
	}{
		{"reference_images", request.ReferenceImages, imageLimit},
		{"reference_videos", request.ReferenceVideos, otherLimit},
		{"reference_audios", request.ReferenceAudios, otherLimit},
	} {
		if len(group.items) > group.limit {
			return service.TaskErrorWrapperLocal(fmt.Errorf("%s allows at most %d items", group.name, group.limit), "invalid_media", http.StatusBadRequest)
		}
		if _, exists := fields[group.name]; !exists {
			continue
		}
		urls := make([]string, 0, len(group.items))
		for _, raw := range group.items {
			value, err := lxmoneSeedanceMediaURL(raw)
			if err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_media", http.StatusBadRequest)
			}
			urls = append(urls, value)
		}
		normalizedMedia[group.name] = urls
	}
	for _, pair := range [][2]string{{"input_reference", "image"}, {"image_end", "end_image_url"}} {
		var previous string
		for _, name := range pair {
			if raw, exists := fields[name]; exists {
				value, err := lxmoneSeedanceMediaURL(raw)
				if err != nil {
					return service.TaskErrorWrapperLocal(err, "invalid_media", http.StatusBadRequest)
				}
				if previous != "" && previous != value {
					return service.TaskErrorWrapperLocal(fmt.Errorf("%s and %s must agree", pair[0], pair[1]), "invalid_media", http.StatusBadRequest)
				}
				previous = value
				normalizedMedia[name] = value
			}
		}
	}
	validDuration := duration >= 4 && duration <= 15
	switch upstreamModel {
	case "seedance-2.5-pro":
		validDuration = duration >= 4 && duration <= 30
	case "seedance-2-fast", "seedance-2-mini":
		validDuration = duration == 5 || duration == 10
	}
	if !validDuration || duration > relaycommon.MaxTaskDurationSeconds {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported duration for %s", upstreamModel), "invalid_duration", http.StatusBadRequest)
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
	if resolution != "480p" && resolution != "720p" && resolution != "1080p" && resolution != "4k" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported resolution"), "invalid_resolution", http.StatusBadRequest)
	}
	switch upstreamModel {
	case "seedance-2-fast", "seedance-2-mini":
		if resolution == "4k" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("4K is not supported by this model"), "invalid_resolution", http.StatusBadRequest)
		}
		if resolution == "1080p" {
			resolution = "720p"
		}
	case "seedance-2.5-pro":
		if resolution == "4k" {
			resolution = "1080p"
		}
	}
	aspectRatio := strings.TrimSpace(request.AspectRatio)
	if request.Ratio != "" && aspectRatio != "" && request.Ratio != aspectRatio {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio and aspect_ratio must agree"), "invalid_request", http.StatusBadRequest)
	}
	if aspectRatio == "" {
		aspectRatio = request.Ratio
	}
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}
	if aspectRatio != "16:9" && aspectRatio != "9:16" && aspectRatio != "1:1" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported aspect_ratio"), "invalid_request", http.StatusBadRequest)
	}
	hasFirst := len(request.InputReference) > 0 || len(request.Image) > 0
	hasLast := len(request.ImageEnd) > 0 || len(request.EndImageURL) > 0
	hasReferences := len(request.ReferenceImages)+len(request.ReferenceVideos)+len(request.ReferenceAudios) > 0
	if hasLast && !hasFirst || (hasFirst || hasLast) && hasReferences {
		return service.TaskErrorWrapperLocal(fmt.Errorf("end frame requires a first frame; frame and reference modes cannot be mixed"), "invalid_media", http.StatusBadRequest)
	}
	if upstreamModel != "seedance-2.5-pro" && len(request.ReferenceAudios) > 0 && len(request.ReferenceImages)+len(request.ReferenceVideos) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference audio requires reference images or videos"), "invalid_media", http.StatusBadRequest)
	}
	// Ratios belong to this channel, never to the shared seedance-2.5 model defaults.
	// Missing prices fail before submitting or reserving any quota.
	ratio := info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios[upstreamModel][resolution]
	if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("this channel has no price for the requested Seedance resolution"), "model_price_error", http.StatusBadRequest)
	}
	c.Set("lxmone_seedance_resolution_ratio", ratio)
	c.Set("lxmone_seedance_media", normalizedMedia)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: info.OriginModelName, Prompt: request.Prompt, Seconds: strconv.Itoa(duration), Duration: duration, Size: resolution})
	info.Action = constant.TaskActionTextGenerate
	if hasFirst || hasReferences {
		info.Action = constant.TaskActionGenerate
	}
	if info.TaskRelayInfo != nil {
		info.TaskRelayInfo.Video = &relaycommon.TaskVideoProperties{Provider: "lxmone-seedance", Duration: duration, Resolution: resolution, Ratio: aspectRatio, InputImageCount: len(request.ReferenceImages)}
	}
	return validateVideoWebhook(c)
}

func isLxmoneSeedanceTask(task *model.Task) bool {
	return task != nil && task.Properties.Video != nil && task.Properties.Video.Provider == "lxmone-seedance"
}
