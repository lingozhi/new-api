package sora

import (
	"fmt"
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// Aijiau accepts a flat payload; public requests use the official nested shape.
func buildAijiauWanRequest(prompt string, media []wanMedia, parameters wanVideoParameters, duration int, resolution, ratio string) map[string]any {
	body := map[string]any{"model": "wan3.0-video", "prompt": prompt, "duration": duration, "seconds": strconv.Itoa(duration), "resolution": resolution, "aspect_ratio": ratio, "audio": true, "seed": int64(-1), "prompt_extend": true, "watermark": false}
	if len(media) > 0 {
		body["media"] = media
	}
	if parameters.Audio != nil {
		body["audio"] = *parameters.Audio
	}
	if parameters.Seed != nil {
		body["seed"] = *parameters.Seed
	}
	if parameters.PromptExtend != nil {
		body["prompt_extend"] = *parameters.PromptExtend
	}
	if parameters.Watermark != nil {
		body["watermark"] = *parameters.Watermark
	}
	return body
}

// Automatic duration reserves the maximum output, then releases the unused
// portion using the provider's delivered duration and the saved price snapshot.
func settleWanAutomaticDuration(task *model.Task, result *relaycommon.TaskInfo) int {
	if result.Status != model.TaskStatusSuccess {
		return 0
	}
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.ModelPrice <= 0 || billing.GroupRatio <= 0 {
		return 0
	}
	var response struct {
		Video *struct {
			Duration float64 `json:"duration"`
		} `json:"video"`
	}
	seconds := float64(2)
	if err := common.Unmarshal(task.Data, &response); err == nil && response.Video != nil && response.Video.Duration > 0 && !math.IsNaN(response.Video.Duration) && !math.IsInf(response.Video.Duration, 0) {
		seconds = math.Min(math.Max(response.Video.Duration, 2), 30)
	} else {
		// Missing provider metadata must never turn the maximum reserve into a
		// maximum charge. Charge the minimum and surface the accounting anomaly.
		common.SysError(fmt.Sprintf("Wan automatic task %s missing delivered duration; settling minimum 2 seconds and releasing reserve", task.TaskID))
	}
	price := &types.PriceData{}
	for name, ratio := range billing.OtherRatios {
		if name != "seconds" {
			price.AddOtherRatio(name, ratio)
		}
	}
	quota, clamp := common.QuotaFromFloatChecked(price.ApplyOtherRatiosToFloat(billing.ModelPrice * common.QuotaPerUnit * billing.GroupRatio * seconds))
	task.PrivateData.FinalQuotaClamp = clamp
	return quota
}
