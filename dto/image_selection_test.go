package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGPTImage2SupplierAliasCombinationMatrix(t *testing.T) {
	allRatios := []string{
		"auto", "1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5",
		"16:9", "9:16", "2:1", "1:2", "3:1", "1:3", "21:9", "9:21",
	}
	tests := []struct {
		model       string
		resolution  string
		unsupported map[string]bool
	}{
		{model: "gpt-image-2-text-to-image", resolution: "1K"},
		{model: "gpt-image-2-text-to-image", resolution: "2K", unsupported: map[string]bool{"auto": true, "5:4": true, "4:5": true, "3:1": true, "1:3": true, "9:21": true}},
		{model: "gpt-image-2-text-to-image", resolution: "4K", unsupported: map[string]bool{"auto": true, "1:1": true, "5:4": true, "4:5": true, "3:1": true, "1:3": true, "9:21": true}},
		{model: "gpt-image-2-image-to-image", resolution: "1K"},
		{model: "gpt-image-2-image-to-image", resolution: "2K", unsupported: map[string]bool{"auto": true, "5:4": true, "4:5": true}},
		{model: "gpt-image-2-image-to-image", resolution: "4K", unsupported: map[string]bool{"auto": true, "1:1": true, "5:4": true, "4:5": true}},
	}

	for _, tt := range tests {
		for _, ratio := range allRatios {
			name := tt.model + "/" + tt.resolution + "/" + ratio
			t.Run(name, func(t *testing.T) {
				request := &ImageRequest{
					Model:  tt.model,
					Prompt: "compatibility test",
					Extra: map[string]json.RawMessage{
						"resolution":   json.RawMessage(`"` + tt.resolution + `"`),
						"aspect_ratio": json.RawMessage(`"` + ratio + `"`),
					},
				}
				requirement, err := ResolveImageSelectionRequirementWithModelDefaults(request, tt.model, ImageOperationGeneration)
				if tt.unsupported[ratio] {
					require.Error(t, err)
					assert.Contains(t, err.Error(), "not supported")
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tt.resolution, requirement.Resolution)
				assert.Equal(t, ratio, requirement.AspectRatio)
				assert.Empty(t, requirement.Size)
			})
		}
	}
}

func TestResolveGPTImage2SupplierAliasDefaultsToAutoOneK(t *testing.T) {
	request := &ImageRequest{Model: "gpt-image-2-text-to-image", Prompt: "compatibility test"}

	requirement, err := ResolveImageSelectionRequirementWithModelDefaults(request, request.Model, ImageOperationGeneration)

	require.NoError(t, err)
	assert.Equal(t, "1K", requirement.Resolution)
	assert.Equal(t, "auto", requirement.AspectRatio)
}

func TestResolveImageSelectionRequirementCanonicalizesGPTImageVariant(t *testing.T) {
	request := &ImageRequest{
		Model:   "gpt-image-2",
		Quality: "LOW",
		Extra: map[string]json.RawMessage{
			"resolution":   json.RawMessage(`"4k"`),
			"aspect_ratio": json.RawMessage(`"16:9"`),
		},
	}

	requirement, err := request.NormalizeImageSelectionRequirement("gpt-image-2", ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, ImageSelectionRequirement{
		Operation:   ImageOperationGeneration,
		Resolution:  "4K",
		AspectRatio: "16:9",
		Size:        "3840x2160",
		Quality:     "low",
		N:           1,
	}, *requirement)
	assert.Equal(t, "4K", request.GetTokenCountMeta().ImageResolution)
}

func TestResolveImageSelectionRequirementIncludesOutputContract(t *testing.T) {
	n := uint(2)
	request := &ImageRequest{
		Model:        "gpt-image-2",
		N:            &n,
		OutputFormat: json.RawMessage(`"JPG"`),
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, "jpeg", requirement.OutputFormat)
	assert.Equal(t, uint(2), requirement.N)
}

func TestResolveImageSelectionRequirementIncludesRoutingInputs(t *testing.T) {
	watermark := false
	request := &ImageRequest{
		Model:             "gpt-image-2",
		Images:            json.RawMessage(`["https://example.com/one.png","https://example.com/two.png"]`),
		Image:             json.RawMessage(`"https://example.com/legacy.png"`),
		Mask:              json.RawMessage(`"https://example.com/mask.png"`),
		Watermark:         &watermark,
		OutputCompression: json.RawMessage(`0`),
		Background:        json.RawMessage(`"transparent"`),
		Moderation:        json.RawMessage(`"low"`),
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationEdit)
	require.NoError(t, err)
	assert.Equal(t, 2, requirement.ReferenceImageCount)
	assert.True(t, requirement.HasMask)
	assert.Equal(t, []string{"background", "moderation", "output_compression", "watermark"}, requirement.OptionalParameters)
}

func TestResolveImageSelectionRequirementUsesLegacySingularImageFallback(t *testing.T) {
	request := &ImageRequest{
		Model: "gpt-image-2",
		Image: json.RawMessage(`"https://example.com/legacy.png"`),
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationEdit)
	require.NoError(t, err)
	assert.Equal(t, 1, requirement.ReferenceImageCount)
	assert.False(t, requirement.HasMask)
	assert.Empty(t, requirement.OptionalParameters)
}

func TestResolveImageSelectionRequirementInfersVariantFromLegacySize(t *testing.T) {
	request := &ImageRequest{Model: "gpt-image-2", Size: "2880X2880"}

	requirement, err := ResolveImageSelectionRequirementWithModelDefaults(request, request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, "4K", requirement.Resolution)
	assert.Equal(t, "1:1", requirement.AspectRatio)
	assert.Equal(t, "2880x2880", requirement.Size)
}

func TestResolveImageSelectionRequirementRejectsConflictingAliases(t *testing.T) {
	request := &ImageRequest{
		Model: "gpt-image-2",
		Size:  "1024x1024",
		Extra: map[string]json.RawMessage{
			"resolution": json.RawMessage(`"4K"`),
		},
	}

	_, err := ResolveImageSelectionRequirementWithModelDefaults(request, request.Model, ImageOperationGeneration)
	require.ErrorContains(t, err, "conflicts with resolution")
}

func TestResolveImageSelectionRequirementUsesOneKBillingDefault(t *testing.T) {
	request := &ImageRequest{Model: "gpt-image-2"}

	requirement, err := request.NormalizeImageSelectionRequirement(request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, "1K", requirement.Resolution)
	assert.Empty(t, requirement.AspectRatio)
	assert.Empty(t, requirement.Size)
	assert.Equal(t, "1K", request.GetTokenCountMeta().ImageResolution)
}

func TestResolveImageSelectionRequirementAcceptsFlashHalfKResolution(t *testing.T) {
	request := &ImageRequest{
		Model: "gemini-3.1-flash-image-preview",
		Extra: map[string]json.RawMessage{
			"resolution": json.RawMessage(`"512"`),
		},
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, "512", requirement.Resolution)
}

func TestResolveImageSelectionRequirementLeavesProfileSpecificSizeUntouched(t *testing.T) {
	request := &ImageRequest{
		Model: "gpt-image-2",
		Size:  "1254x1254",
		Extra: map[string]json.RawMessage{
			"resolution": json.RawMessage(`"1K"`),
		},
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, "1K", requirement.Resolution)
	assert.Equal(t, "1254x1254", requirement.Size)
	assert.Empty(t, requirement.AspectRatio)
}

func TestResolveImageSelectionRequirementCapturesProviderCountsAndCanonicalParameters(t *testing.T) {
	request := &ImageRequest{
		Model: "provider-image-model",
		Extra: map[string]json.RawMessage{
			"batch_size":       json.RawMessage(`2`),
			"generationConfig": json.RawMessage(`{"candidateCount":2}`),
			"negativePrompt":   json.RawMessage(`"fog"`),
		},
	}

	requirement, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
	require.NoError(t, err)
	assert.Equal(t, uint(2), requirement.N)
	assert.Contains(t, requirement.OptionalParameters, "batch_size")
	assert.Contains(t, requirement.OptionalParameters, "generation_config")
	assert.Contains(t, requirement.OptionalParameters, "negative_prompt")
	assert.JSONEq(t, `{"candidateCount":2}`, string(requirement.OptionalValues["generation_config"]))
}

func TestResolveImageSelectionRequirementRejectsCollidingProviderParameterAliases(t *testing.T) {
	request := &ImageRequest{
		Model: "provider-image-model",
		Extra: map[string]json.RawMessage{
			"negativePrompt":  json.RawMessage(`"fog"`),
			"negative_prompt": json.RawMessage(`"rain"`),
		},
	}

	_, err := ResolveImageSelectionRequirement(request, request.Model, ImageOperationGeneration)
	require.ErrorContains(t, err, "aliases collide")
}
