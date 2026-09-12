package common

import (
	"net/url"
	"strings"
)

func IsAijiauVideoBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), "tokens.aijiakefu.com")
}

// OpenAIVideoBaseURL keeps submission, polling and content downloads on the
// same provider endpoint. Aijiau uses /videos/generations for all three.
func OpenAIVideoBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if IsAijiauVideoBaseURL(baseURL) {
		return strings.TrimSuffix(baseURL, "/v1") + "/v1/videos/generations"
	}
	return baseURL + "/v1/videos"
}
