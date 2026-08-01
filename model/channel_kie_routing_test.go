package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func TestKIEJobsImageRoutingRequiresAdvancedCustomChannel(t *testing.T) {
	t.Parallel()

	routing := &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{
			{
				Model:              "gpt-image-2",
				Protocol:           dto.ImageRoutingProtocolKIEJobs,
				UpstreamPath:       "/api/v1/jobs/createTask",
				Operations:         []dto.ImageOperation{dto.ImageOperationGeneration, dto.ImageOperationEdit},
				MaxOutputImages:    1,
				MaxReferenceImages: 16,
				VerificationStatus: dto.ImageRoutingVerificationDocsClaimed,
			},
		},
	}

	advanced := &Channel{Type: constant.ChannelTypeAdvancedCustom}
	advanced.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
			{
				IncomingPath: "/v1/jobs",
				UpstreamPath: "/api/v1/jobs/createTask",
				Converter:    "none",
				Models:       []string{"gpt-image-2"},
			},
		}},
		ImageRouting: routing,
	})
	require.NoError(t, advanced.ValidateSettings())

	openAI := &Channel{Type: constant.ChannelTypeOpenAI}
	openAI.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: routing})
	require.ErrorContains(t, openAI.ValidateSettings(), "incompatible with channel type")
}
