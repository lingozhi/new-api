package sora

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type wanMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type wanVideoInput struct {
	Prompt *string    `json:"prompt,omitempty"`
	Media  []wanMedia `json:"media,omitempty"`
}

type wanVideoParameters struct {
	Resolution   *string `json:"resolution,omitempty"`
	Ratio        *string `json:"ratio,omitempty"`
	Duration     *int    `json:"duration,omitempty"`
	Audio        *bool   `json:"audio,omitempty"`
	Seed         *int64  `json:"seed,omitempty"`
	PromptExtend *bool   `json:"prompt_extend,omitempty"`
	Watermark    *bool   `json:"watermark,omitempty"`
}

type wanVideoRequest struct {
	Model         string              `json:"model"`
	Input         *wanVideoInput      `json:"input"`
	Parameters    *wanVideoParameters `json:"parameters,omitempty"`
	WebhookURL    *string             `json:"webhook_url,omitempty"`
	WebhookSecret *string             `json:"webhook_secret,omitempty"`
}

// Validate every object before constructing the provider payload so aliases and
// unknown billing parameters cannot bypass the typed request contract.
func validateWanObjectFields(value any, path string, allowed ...string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", path)
	}
	for key, value := range object {
		if value == nil || !common.StringsContains(allowed, key) {
			return fmt.Errorf("unsupported or null field %s.%s; omit unused fields", path, key)
		}
	}
	return nil
}

func validateWanMediaURL(media wanMedia) error {
	if strings.HasPrefix(media.URL, "data:") {
		if media.Type != "reference_image" && media.Type != "first_frame" && media.Type != "last_frame" {
			return fmt.Errorf("Base64 data URLs are only supported for images")
		}
		header, data, ok := strings.Cut(media.URL, ",")
		if !ok || !common.StringsContains([]string{"data:image/jpeg;base64", "data:image/png;base64", "data:image/bmp;base64", "data:image/webp;base64"}, header) {
			return fmt.Errorf("image data URL must contain a supported image MIME type and Base64 data")
		}
		const maxImageBytes = 20 * 1024 * 1024
		if len(data) > base64.StdEncoding.EncodedLen(maxImageBytes) {
			return fmt.Errorf("image exceeds 20 MB")
		}
		size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
		if err != nil || size == 0 || size > maxImageBytes {
			return fmt.Errorf("invalid or oversized Base64 image")
		}
		return nil
	}
	parsed, err := url.Parse(media.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("media requires a public HTTP(S) URL without credentials; DashScope OSS URLs are not available on this channel")
	}
	return nil
}

// Only the website alias is mapped. Prime must retain its upstream identity.
func wanUpstreamModel(modelName string) string {
	if modelName == "wan3.0" {
		return "wan3.0-video"
	}
	return modelName
}
