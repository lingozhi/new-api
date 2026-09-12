package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWanCatalogAdvertisesVideoEndpoint(t *testing.T) {
	for _, name := range []string{"wan3.0", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(name, func(t *testing.T) {
			endpoints := getPricingEndpointTypesForAbility(AbilityWithChannel{Ability: Ability{Model: name}, ChannelType: constant.ChannelTypeOpenAI}, nil)
			assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, endpoints)
		})
	}
}
