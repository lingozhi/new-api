package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAIVideoBaseURL(t *testing.T) {
	for _, tc := range []struct{ baseURL, expected string }{
		{"https://tokens.aijiakefu.com", "https://tokens.aijiakefu.com/v1/videos/generations"},
		{"https://tokens.aijiakefu.com/", "https://tokens.aijiakefu.com/v1/videos/generations"},
		{"https://tokens.aijiakefu.com/v1/", "https://tokens.aijiakefu.com/v1/videos/generations"},
		{"https://TOKENS.AIJIAKEFU.COM", "https://TOKENS.AIJIAKEFU.COM/v1/videos/generations"},
		{"https://tokens.aijiakefu.com.example", "https://tokens.aijiakefu.com.example/v1/videos"},
		{"https://lxmone.xyz", "https://lxmone.xyz/v1/videos"},
		{"https://api.openai.com", "https://api.openai.com/v1/videos"},
		{"https://gateway.example/prefix/", "https://gateway.example/prefix/v1/videos"},
	} {
		t.Run(tc.baseURL, func(t *testing.T) {
			assert.Equal(t, tc.expected, OpenAIVideoBaseURL(tc.baseURL))
		})
	}
}
