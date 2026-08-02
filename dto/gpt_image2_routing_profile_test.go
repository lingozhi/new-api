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
		Resolution:  "2K",
		AspectRatio: "auto",
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
	assert.False(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
		Operation:   dto.ImageOperationGeneration,
		Resolution:  "2K",
		AspectRatio: "auto",
		Size:        "auto",
		N:           1,
	}))
	assert.False(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
		Operation:   dto.ImageOperationGeneration,
		Resolution:  "4K",
		AspectRatio: "3:2",
		N:           1,
	}))

	negativeCases := []struct {
		name   string
		mutate func(*dto.ImageRoutingProfile)
	}{
		{
			name: "another model cannot omit defaults",
			mutate: func(candidate *dto.ImageRoutingProfile) {
				candidate.Model = "future-image-model"
			},
		},
		{
			name: "image editing cannot omit defaults",
			mutate: func(candidate *dto.ImageRoutingProfile) {
				candidate.Protocol = dto.ImageRoutingProtocolImagesEdits
				candidate.UpstreamPath = "/v1/images/edits"
				candidate.Operations = []dto.ImageOperation{dto.ImageOperationEdit}
				for index := range candidate.AllowedCombinations {
					candidate.AllowedCombinations[index].Operation = dto.ImageOperationEdit
				}
			},
		},
		{
			name: "mixed operations cannot omit defaults",
			mutate: func(candidate *dto.ImageRoutingProfile) {
				candidate.Operations = []dto.ImageOperation{dto.ImageOperationGeneration, dto.ImageOperationEdit}
			},
		},
		{
			name: "missing auto size default cannot omit defaults",
			mutate: func(candidate *dto.ImageRoutingProfile) {
				candidate.DefaultSize = ""
			},
		},
	}
	for _, testCase := range negativeCases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := profile
			candidate.Operations = append([]dto.ImageOperation(nil), profile.Operations...)
			candidate.AllowedCombinations = append(
				[]dto.ImageRoutingCombination(nil),
				profile.AllowedCombinations...,
			)
			testCase.mutate(&candidate)
			err := (&dto.ImageRoutingConfig{
				Version:  dto.ImageRoutingVersion1,
				Profiles: []dto.ImageRoutingProfile{candidate},
			}).Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, "default_resolution is required")
		})
	}

	t.Run("mixed operations cannot restore the contract auto sentinel with explicit defaults", func(t *testing.T) {
		candidate := profile
		candidate.Operations = []dto.ImageOperation{dto.ImageOperationGeneration, dto.ImageOperationEdit}
		candidate.DefaultResolution = "1K"
		candidate.DefaultAspectRatio = "auto"
		candidate.AllowedCombinations = append(
			append([]dto.ImageRoutingCombination(nil), profile.AllowedCombinations...),
			dto.ImageRoutingCombination{
				Operation:   dto.ImageOperationEdit,
				Resolution:  "1K",
				AspectRatio: "1:1",
				Size:        "1024x1024",
			},
		)
		err := (&dto.ImageRoutingConfig{
			Version:  dto.ImageRoutingVersion1,
			Profiles: []dto.ImageRoutingProfile{candidate},
		}).Validate()
		require.Error(t, err)
		assert.ErrorContains(t, err, "exact size")
	})

	t.Run("contract auto sentinel cannot carry output controls", func(t *testing.T) {
		candidate := profile
		candidate.DefaultResolution = "1K"
		candidate.DefaultAspectRatio = "auto"
		candidate.OutputFormats = []string{"png"}
		candidate.DefaultOutputFormat = "png"
		candidate.AllowedCombinations = append(
			[]dto.ImageRoutingCombination(nil),
			profile.AllowedCombinations...,
		)
		candidate.AllowedCombinations[0].OutputFormat = "png"
		err := (&dto.ImageRoutingConfig{
			Version:  dto.ImageRoutingVersion1,
			Profiles: []dto.ImageRoutingProfile{candidate},
		}).Validate()
		require.Error(t, err)
		assert.ErrorContains(t, err, "exact size")
	})

	for _, testCase := range []struct {
		name        string
		configure   func(*dto.ImageRoutingProfile)
		requirement dto.ImageSelectionRequirement
	}{
		{
			name: "contract auto sentinel cannot borrow a declared quality",
			configure: func(candidate *dto.ImageRoutingProfile) {
				candidate.Qualities = []string{"high"}
				candidate.DefaultQuality = "high"
				for index := 1; index < len(candidate.AllowedCombinations); index++ {
					candidate.AllowedCombinations[index].Quality = "high"
				}
			},
			requirement: dto.ImageSelectionRequirement{
				Operation: dto.ImageOperationGeneration,
				Size:      "auto",
				Quality:   "high",
				N:         1,
			},
		},
		{
			name: "contract auto sentinel cannot borrow a declared output format",
			configure: func(candidate *dto.ImageRoutingProfile) {
				candidate.OutputFormats = []string{"png"}
				candidate.DefaultOutputFormat = "png"
				for index := 1; index < len(candidate.AllowedCombinations); index++ {
					candidate.AllowedCombinations[index].OutputFormat = "png"
				}
			},
			requirement: dto.ImageSelectionRequirement{
				Operation:    dto.ImageOperationGeneration,
				Size:         "auto",
				OutputFormat: "png",
				N:            1,
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := profile
			candidate.AllowedCombinations = append(
				[]dto.ImageRoutingCombination(nil),
				profile.AllowedCombinations...,
			)
			testCase.configure(&candidate)
			config := &dto.ImageRoutingConfig{
				Version:  dto.ImageRoutingVersion1,
				Profiles: []dto.ImageRoutingProfile{candidate},
			}

			err := config.Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, "default image tuple")
			assert.False(t, config.Supports("gpt-image-2", testCase.requirement))
		})
	}

	for _, testCase := range []struct {
		name        string
		configure   func(*dto.ImageRoutingProfile)
		requirement dto.ImageSelectionRequirement
	}{
		{
			name: "exact auto geometry cannot borrow a declared quality",
			configure: func(candidate *dto.ImageRoutingProfile) {
				candidate.Qualities = []string{"high"}
				candidate.DefaultQuality = "high"
				for index := 1; index < len(candidate.AllowedCombinations); index++ {
					candidate.AllowedCombinations[index].Quality = "high"
				}
			},
			requirement: dto.ImageSelectionRequirement{
				Operation:   dto.ImageOperationGeneration,
				Resolution:  "1K",
				AspectRatio: "auto",
				Size:        "auto",
				Quality:     "high",
				N:           1,
			},
		},
		{
			name: "exact auto geometry cannot borrow a declared output format",
			configure: func(candidate *dto.ImageRoutingProfile) {
				candidate.OutputFormats = []string{"png"}
				candidate.DefaultOutputFormat = "png"
				for index := 1; index < len(candidate.AllowedCombinations); index++ {
					candidate.AllowedCombinations[index].OutputFormat = "png"
				}
			},
			requirement: dto.ImageSelectionRequirement{
				Operation:    dto.ImageOperationGeneration,
				Resolution:   "1K",
				AspectRatio:  "auto",
				Size:         "auto",
				OutputFormat: "png",
				N:            1,
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := profile
			candidate.DefaultResolution = "1K"
			candidate.DefaultAspectRatio = "auto"
			candidate.AllowedCombinations = append(
				[]dto.ImageRoutingCombination(nil),
				profile.AllowedCombinations...,
			)
			candidate.AllowedCombinations[0] = dto.ImageRoutingCombination{
				Operation:   dto.ImageOperationGeneration,
				Resolution:  "1K",
				AspectRatio: "auto",
				Size:        "auto",
			}
			testCase.configure(&candidate)
			config := &dto.ImageRoutingConfig{
				Version:  dto.ImageRoutingVersion1,
				Profiles: []dto.ImageRoutingProfile{candidate},
			}

			err := config.Validate()
			require.Error(t, err)
			assert.ErrorContains(t, err, "default image tuple")
			assert.False(t, config.Supports("gpt-image-2", testCase.requirement))
		})
	}
}

func TestGPTImage2ProductionProfileAllowsEditAutoTupleAlongsideGeneration(t *testing.T) {
	profile := dto.ImageRoutingProfile{
		Model:              "gpt-image-2",
		Protocol:           dto.ImageRoutingProtocolImagesGenerations,
		UpstreamPath:       "/v1/images/generations",
		Operations:         []dto.ImageOperation{dto.ImageOperationGeneration, dto.ImageOperationEdit},
		Resolutions:        []string{"1K", "2K"},
		AspectRatios:       []string{"auto", "1:1"},
		Sizes:              []string{"auto", "1024x1024", "1440x1440"},
		DefaultResolution:  "1K",
		DefaultAspectRatio: "auto",
		DefaultSize:        "auto",
		MaxOutputImages:    1,
		MaxReferenceImages: 16,
		AllowedCombinations: []dto.ImageRoutingCombination{
			{Operation: dto.ImageOperationGeneration, Size: "auto"},
			{Operation: dto.ImageOperationEdit, Size: "auto"},
			{Operation: dto.ImageOperationGeneration, Resolution: "1K", AspectRatio: "1:1", Size: "1024x1024"},
			{Operation: dto.ImageOperationEdit, Resolution: "1K", AspectRatio: "1:1", Size: "1024x1024"},
			{Operation: dto.ImageOperationGeneration, Resolution: "2K", AspectRatio: "1:1", Size: "1440x1440"},
			{Operation: dto.ImageOperationEdit, Resolution: "2K", AspectRatio: "1:1", Size: "1440x1440"},
		},
		VerificationStatus: dto.ImageRoutingVerificationProductionVerified,
	}
	config := &dto.ImageRoutingConfig{Version: dto.ImageRoutingVersion1, Profiles: []dto.ImageRoutingProfile{profile}}
	require.NoError(t, config.Validate())
	assert.True(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
		Operation: dto.ImageOperationEdit, Size: "auto", ReferenceImageCount: 1, N: 1,
	}))
	assert.True(t, config.Supports("gpt-image-2", dto.ImageSelectionRequirement{
		Operation: dto.ImageOperationEdit, Resolution: "2K", AspectRatio: "1:1", Size: "1440x1440", ReferenceImageCount: 1, N: 1,
	}))
}
