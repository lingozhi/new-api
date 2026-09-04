package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRoutingForwardsUndeclaredQualityOnImagesEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name      string
		protocol  ImageRoutingProtocol
		operation ImageOperation
		size      string
		aspect    string
		path      string
		allowed   bool
	}{
		{"generations", ImageRoutingProtocolImagesGenerations, ImageOperationGeneration, "1024x1024", "1:1", "/v1/images/generations", true},
		{"edits", ImageRoutingProtocolImagesEdits, ImageOperationEdit, "1024x1024", "1:1", "/v1/images/edits", true},
		{"provider-auto", ImageRoutingProtocolImagesGenerations, ImageOperationGeneration, "auto", "auto", "/v1/images/generations", true},
		{"native-responses", ImageRoutingProtocolResponsesSSE, ImageOperationGeneration, "1024x1024", "1:1", "/v1/responses", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := ImageRoutingProfile{
				Model: "gpt-image-2", Protocol: tc.protocol, UpstreamPath: tc.path, Operations: []ImageOperation{tc.operation},
				Resolutions: []string{"1K"}, AspectRatios: []string{tc.aspect}, Sizes: []string{tc.size},
				VerificationStatus:  ImageRoutingVerificationProductionVerified,
				AllowedCombinations: []ImageRoutingCombination{{Operation: tc.operation, Resolution: "1K", AspectRatio: tc.aspect, Size: tc.size}},
			}
			config := &ImageRoutingConfig{Version: ImageRoutingVersion1, Profiles: []ImageRoutingProfile{profile}}
			require.NoError(t, config.Validate())
			for _, quality := range []string{"auto", "low", "medium", "high"} {
				request := ImageSelectionRequirement{Operation: tc.operation, Size: tc.size, Quality: quality, N: 1}
				assert.Equal(t, tc.allowed, config.Supports(profile.Model, request), quality)
				assert.Equal(t, quality, request.Quality)
			}
			defaults, err := profile.ApplyDefaults(ImageSelectionRequirement{Operation: tc.operation, N: 1})
			require.NoError(t, err)
			assert.Empty(t, defaults.Quality, "omitted quality must remain delegated to the provider")
		})
	}
}
