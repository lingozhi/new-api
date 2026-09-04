package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderImageSizesRetainRoutingAndBillingTier(t *testing.T) {
	for _, test := range []struct{ resolution, aspect, size string }{
		{"2K", "1:1", "1440x1440"}, {"4K", "1:1", "2160x2160"},
		{"2K", "16:9", "2560x1440"}, {"4K", "16:9", "3840x2160"},
		{"2K", "9:16", "1152x2048"}, {"4K", "9:16", "2160x3840"},
		{"2K", "4:3", "1920x1440"}, {"4K", "4:3", "2880x2160"},
		{"2K", "3:4", "1440x1920"}, {"4K", "3:4", "2160x2880"},
		{"2K", "3:2", "2160x1440"}, {"4K", "3:2", "3240x2160"},
		{"2K", "2:3", "1440x2160"}, {"4K", "2:3", "2160x3240"},
		{"2K", "21:9", "2560x1097"}, {"4K", "21:9", "3840x1646"},
		{"1K", "auto", "auto"}, {"2K", "auto", "auto"}, {"4K", "auto", "auto"},
		{"1K", "9:16", "9:16"}, {"2K", "9:16", "9:16"}, {"4K", "9:16", "9:16"},
	} {
		t.Run(test.resolution+"/"+test.size, func(t *testing.T) {
			profile := ImageRoutingProfile{
				Model: "gpt-image-2", Protocol: ImageRoutingProtocolImagesGenerations,
				UpstreamPath: "/v1/images/generations", Operations: []ImageOperation{ImageOperationGeneration},
				Resolutions: []string{test.resolution}, AspectRatios: []string{test.aspect}, Sizes: []string{test.size},
				VerificationStatus: ImageRoutingVerificationProductionVerified,
				AllowedCombinations: []ImageRoutingCombination{{Operation: ImageOperationGeneration,
					Resolution: test.resolution, AspectRatio: test.aspect, Size: test.size}},
			}
			config := &ImageRoutingConfig{Version: ImageRoutingVersion1, Profiles: []ImageRoutingProfile{profile}}
			require.NoError(t, config.Validate())
			request := &ImageRequest{Model: profile.Model, Size: test.size}
			selection, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
			require.NoError(t, err)
			require.True(t, config.Supports(profile.Model, selection))
			selected, err := profile.ApplyDefaults(selection)
			require.NoError(t, err)
			require.NoError(t, request.SetImageSelectionRequirement(selected))
			assert.Equal(t, test.size, selected.Size)
			assert.Equal(t, test.aspect, selected.AspectRatio)
			assert.Equal(t, test.resolution, request.GetTokenCountMeta().ImageResolution)
			selected.Resolution = "8K"
			assert.False(t, config.Supports(profile.Model, selected), "a different billing tier must not use this channel")
			selection.Size = "999x999"
			assert.False(t, config.Supports(profile.Model, selection), "sizes still require a matching provider profile")
		})
	}
}

func TestImageRatioSizeValidation(t *testing.T) {
	for _, size := range []string{"0:16", "9:0", "-9:16", "9:16:1", "9.5:16"} {
		_, err := (ImageSelectionRequirement{Operation: ImageOperationGeneration, Size: size}).Normalize()
		assert.Error(t, err, size)
	}
	_, err := (ImageSelectionRequirement{Operation: ImageOperationGeneration, Size: "9:16", N: MaxImageN + 1}).Normalize()
	require.ErrorContains(t, err, "n must be")
	_, err = (ImageSelectionRequirement{Operation: ImageOperationGeneration, Size: "9:16", AspectRatio: "16:9"}).Normalize()
	require.ErrorContains(t, err, "conflicts with aspect_ratio")
}
