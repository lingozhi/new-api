package sora

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// Aijiau exposes one Wan model with typed media instead of separate workflow
// models and reference lists. Match its media-studio request contract.
func buildAijiauWanRequest(body map[string]any) error {
	var media []wanMedia
	if raw, exists := body["media"]; exists {
		encoded, err := common.Marshal(raw)
		if err != nil {
			return err
		}
		if err := common.Unmarshal(encoded, &media); err != nil {
			return err
		}
	}
	for _, field := range []string{"reference_images", "reference_videos", "reference_audios"} {
		entries, _ := body[field].([]any)
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid %s entry", field)
			}
			mediaType := "reference_image"
			switch field {
			case "reference_images":
				if role, ok := entry["role"].(string); ok && role != "" {
					mediaType = role
				}
			case "reference_videos":
				if _, exists := entry["duration"]; exists {
					return fmt.Errorf("Aijiau reference videos do not accept duration metadata")
				}
				mediaType = "reference_video"
			case "reference_audios":
				mediaType = "reference_audio"
			}
			mediaURL, _ := entry["url"].(string)
			media = append(media, wanMedia{Type: mediaType, URL: mediaURL})
		}
		delete(body, field)
	}
	for i := range media {
		if media[i].Type == "audio" {
			media[i].Type = "reference_audio"
		}
	}
	if len(media) > 0 {
		body["media"] = media
	}
	body["model"] = "wan3.0-video"
	return nil
}
