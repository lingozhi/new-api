package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKIEJobsImageRoutingProfileSupportsGenerationAndEdit(t *testing.T) {
	t.Parallel()

	config := &ImageRoutingConfig{
		Version: ImageRoutingVersion1,
		Profiles: []ImageRoutingProfile{
			{
				Model:              "gpt-image-2",
				Protocol:           ImageRoutingProtocolKIEJobs,
				UpstreamPath:       "/api/v1/jobs/createTask",
				Operations:         []ImageOperation{ImageOperationGeneration, ImageOperationEdit},
				Resolutions:        []string{"1K", "2K", "4K"},
				AspectRatios:       []string{"auto", "1:1", "16:9"},
				MaxOutputImages:    1,
				MaxReferenceImages: 16,
				VerificationStatus: ImageRoutingVerificationDocsClaimed,
			},
		},
	}

	require.NoError(t, config.Validate())
	profile, ok := config.ProfileForModel("gpt-image-2")
	require.True(t, ok)
	require.NotNil(t, profile)
	for _, operation := range []ImageOperation{ImageOperationGeneration, ImageOperationEdit} {
		protocol, path, routeOK := profile.RouteForOperation(operation)
		assert.True(t, routeOK)
		assert.Equal(t, ImageRoutingProtocolKIEJobs, protocol)
		assert.Equal(t, "/api/v1/jobs/createTask", path)
	}
}

func TestKIEJobsImageRoutingProfileRejectsWrongEndpoint(t *testing.T) {
	t.Parallel()

	config := &ImageRoutingConfig{
		Version: ImageRoutingVersion1,
		Profiles: []ImageRoutingProfile{
			{
				Model:              "gpt-image-2",
				Protocol:           ImageRoutingProtocolKIEJobs,
				UpstreamPath:       "/v1/images/generations",
				Operations:         []ImageOperation{ImageOperationGeneration},
				VerificationStatus: ImageRoutingVerificationDocsClaimed,
			},
		},
	}

	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KIE jobs createTask")
}
