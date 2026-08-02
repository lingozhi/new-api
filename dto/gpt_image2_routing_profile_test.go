package dto_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTImage2ProductionProfileValidatesAndRoutesTheCompleteMatrix(t *testing.T) {
	capabilities := common.ImageModelCapabilitiesForModel("gpt-image-2")
	combinations := make([]dto.ImageRoutingCombination, 0, len(capabilities.ResolutionAspectVariants))
	for _, variant := range capabilities.ResolutionAspectVariants {
		combinations = append(combinations, dto.ImageRoutingCombination{
			Operation:   dto.ImageOperationGeneration,
			Resolution:  variant.Resolution,
			AspectRatio: variant.AspectRatio,
			Size:        variant.Size,
		})
	}
	require.Len(t, combinations, 36)

	config := &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{{
			Model:               "gpt-image-2",
			Protocol:            dto.ImageRoutingProtocolImagesGenerations,
			UpstreamPath:        "/v1/images/generations",
			Operations:          []dto.ImageOperation{dto.ImageOperationGeneration},
			Resolutions:         append([]string(nil), capabilities.Resolutions...),
			AspectRatios:        append([]string(nil), capabilities.AspectRatios...),
			Sizes:               append([]string(nil), capabilities.Sizes...),
			DefaultResolution:   "1K",
			DefaultAspectRatio:  "auto",
			DefaultSize:         "auto",
			MaxOutputImages:     1,
			AllowedCombinations: combinations,
			VerificationStatus:  dto.ImageRoutingVerificationProductionVerified,
		}},
	}

	require.NoError(t, config.Validate())
	for _, combination := range combinations {
		assert.True(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
			Operation:   dto.ImageOperationGeneration,
			Resolution:  combination.Resolution,
			AspectRatio: combination.AspectRatio,
			Size:        combination.Size,
		}), "%s %s %s", combination.Resolution, combination.AspectRatio, combination.Size)
	}
	assert.False(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
		Operation:   dto.ImageOperationGeneration,
		Resolution:  "4K",
		AspectRatio: "3:2",
	}))
}

func TestGPTImage2ProductionProfileAllowsContractAutoTupleWithoutInventingDefaults(t *testing.T) {
	profile := dto.ImageRoutingProfile{
		Model:              "gpt-image-2",
		Protocol:           dto.ImageRoutingProtocolImagesGenerations,
		UpstreamPath:       "/v1/images/generations",
		Operations:         []dto.ImageOperation{dto.ImageOperationGeneration},
		Resolutions:        []string{"1K", "2K"},
		AspectRatios:       []string{"auto", "1:1"},
		Sizes:              []string{"auto", "1024x1024", "1440x1440"},
		DefaultSize:        "auto",
		MaxOutputImages:    1,
		VerificationStatus: dto.ImageRoutingVerificationProductionVerified,
		AllowedCombinations: []dto.ImageRoutingCombination{
			{Operation: dto.ImageOperationGeneration, Size: "auto"},
			{Operation: dto.ImageOperationGeneration, Resolution: "1K", AspectRatio: "1:1", Size: "1024x1024"},
			{Operation: dto.ImageOperationGeneration, Resolution: "2K", AspectRatio: "1:1", Size: "1440x1440"},
		},
	}
	config := &dto.ImageRoutingConfig{
		Version:  dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{profile},
	}

	require.NoError(t, config.Validate())

	auto, err := profile.ApplyDefaults(dto.ImageSelectionRequirement{
		Operation: dto.ImageOperationGeneration,
		Size:      "auto",
		N:         1,
	})
	require.NoError(t, err)
	assert.Equal(t, "", auto.Resolution)
	assert.Equal(t, "", auto.AspectRatio)
	assert.Equal(t, "auto", auto.Size)

	explicitAuto, err := profile.ApplyDefaults(dto.ImageSelectionRequirement{
		Operation:   dto.ImageOperationGeneration,
		Resolution:  "1K",
		AspectRatio: "auto",
		N:           1,
	})
	require.NoError(t, err)
	assert.Equal(t, "1K", explicitAuto.Resolution)
	assert.Equal(t, "auto", explicitAuto.AspectRatio)
	assert.Equal(t, "auto", explicitAuto.Size)
}
