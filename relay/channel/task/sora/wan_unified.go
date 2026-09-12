package sora

import (
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Wan uses the official input/parameters contract with the gateway task endpoints.
func validateUnifiedWanVideoRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" || c.Request.URL.Path != "/v1/videos" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("Wan requires JSON POST /v1/videos"), "invalid_request", http.StatusBadRequest)
	}
	var request wanVideoRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var body map[string]any
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if err := validateWanObjectFields(body, "request", "model", "input", "parameters", "webhook_url", "webhook_secret"); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if request.Model != info.OriginModelName || request.Input == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model and input are required"), "invalid_request", http.StatusBadRequest)
	}
	if err := validateWanObjectFields(body["input"], "input", "prompt", "media"); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if raw, exists := body["parameters"]; exists {
		if err := validateWanObjectFields(raw, "parameters", "resolution", "ratio", "duration", "audio", "seed", "prompt_extend", "watermark"); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}
	prompt := ""
	if request.Input.Prompt != nil {
		// The official API truncates at 20,000 Unicode characters, not bytes.
		chars := []rune(*request.Input.Prompt)
		prompt = string(chars[:min(len(chars), 20000)])
	}
	if strings.TrimSpace(prompt) == "" && len(request.Input.Media) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("input.prompt or input.media is required"), "invalid_request", http.StatusBadRequest)
	}
	if len(request.Input.Media) > 20 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("input.media allows at most 20 items"), "invalid_media", http.StatusBadRequest)
	}
	counts := map[string]int{}
	limits := map[string]int{"first_frame": 1, "last_frame": 1, "reference_image": 10, "reference_video": 5, "reference_audio": 5, "file": 1, "link": 1}
	if entries, ok := body["input"].(map[string]any)["media"].([]any); ok {
		for _, entry := range entries {
			if err := validateWanObjectFields(entry, "input.media[]", "type", "url"); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_media", http.StatusBadRequest)
			}
		}
	}
	for _, media := range request.Input.Media {
		counts[media.Type]++
		if counts[media.Type] > limits[media.Type] {
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported or excessive media type %s", media.Type), "invalid_media", http.StatusBadRequest)
		}
		if err := validateWanMediaURL(media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_media", http.StatusBadRequest)
		}
	}
	frames := counts["first_frame"] + counts["last_frame"]
	if counts["last_frame"] > counts["first_frame"] || (frames > 0 && frames != len(request.Input.Media)) || counts["file"]+counts["link"] > 1 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("last_frame requires first_frame; frames cannot mix with references, file or link; file and link are exclusive"), "invalid_media", http.StatusBadRequest)
	}
	parameters := wanVideoParameters{}
	if request.Parameters != nil {
		parameters = *request.Parameters
	}
	duration, resolution, ratio := 5, "1080P", "adaptive"
	if parameters.Duration != nil {
		duration = *parameters.Duration
	}
	if duration != -1 && (duration < 2 || duration > 30 || duration > relaycommon.MaxTaskDurationSeconds || duration+counts["reference_video"] > 30) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("parameters.duration must be -1 or 2–30; input video seconds plus output seconds must not exceed 30"), "invalid_duration", http.StatusBadRequest)
	}
	if parameters.Resolution != nil {
		resolution = *parameters.Resolution
	}
	if resolution != "480P" && resolution != "720P" && resolution != "1080P" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("parameters.resolution must be 480P, 720P or 1080P"), "invalid_resolution", http.StatusBadRequest)
	}
	if parameters.Ratio != nil {
		ratio = *parameters.Ratio
	}
	if !common.StringsContains([]string{"adaptive", "16:9", "4:3", "1:1", "3:4", "9:16"}, ratio) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("parameters.ratio must be adaptive, 16:9, 4:3, 1:1, 3:4 or 9:16"), "invalid_request", http.StatusBadRequest)
	}
	if parameters.Seed != nil && (*parameters.Seed < -1 || *parameters.Seed > 2147483647) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("parameters.seed must be -1 or 0–2147483647"), "invalid_request", http.StatusBadRequest)
	}
	reserveSeconds := duration
	if duration == -1 {
		reserveSeconds = 30
	}
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: request.Model, Prompt: prompt, Duration: reserveSeconds, Seconds: strconv.Itoa(reserveSeconds), Size: strings.ToLower(resolution)})
	if taskErr := validateVideoWebhook(c); taskErr != nil {
		return taskErr
	}
	info.Action = constant.TaskActionTextGenerate
	if len(request.Input.Media) > 0 {
		info.Action = constant.TaskActionGenerate
	}
	if info.TaskRelayInfo != nil {
		info.TaskRelayInfo.Video = &relaycommon.TaskVideoProperties{Provider: "wan-unified", Resolution: strings.ToLower(resolution), Duration: duration, Ratio: ratio, InputImageCount: counts["reference_image"] + frames}
	}
	c.Set("wan_unified_body", buildAijiauWanRequest(prompt, request.Input.Media, parameters, duration, resolution, ratio))
	return nil
}
