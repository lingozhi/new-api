package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// Show the primary provider's tiers; never rewrite shared model prices.
func lxmonePricingResolutionRatios(model string, abilities []AbilityWithChannel) (map[string]float64, bool) {
	if len(abilities) == 0 {
		return nil, false
	}
	chosen := abilities[0]
	priority := int64(0)
	if chosen.Priority != nil {
		priority = *chosen.Priority
	}
	for _, ability := range abilities[1:] {
		candidatePriority := int64(0)
		if ability.Priority != nil {
			candidatePriority = *ability.Priority
		}
		if candidatePriority > priority || candidatePriority == priority && ability.ChannelId < chosen.ChannelId {
			chosen = ability
			priority = candidatePriority
		}
	}
	if !common.IsLxmoneSeedance(chosen.ChannelBaseURL, model) {
		return nil, false
	}
	var settings dto.ChannelOtherSettings
	if common.UnmarshalJsonStr(chosen.ChannelOtherSettingsJSON, &settings) != nil {
		return nil, true
	}
	ratios := make(map[string]float64)
	for resolution, ratio := range settings.LxmoneSeedanceResolutionRatios[common.LxmoneSeedanceModel(model)] {
		if ratio > 0 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
			ratios[resolution] = ratio
		}
	}
	return ratios, true
}
