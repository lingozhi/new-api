package common

import "github.com/QuantumNous/new-api/constant"

// SeedanceResolutionRatios are relative to the configured 720p per-second price.
// Both billing and the public price catalog use these ratios.
func SeedanceResolutionRatios(model string) map[string]float64 {
	switch model {
	case constant.ArgolinkSeedance25Model:
		return map[string]float64{"480p": 0.077 / 0.17, "720p": 1, "1080p": 0.43 / 0.17}
	case constant.ArgolinkSeedance20Model:
		return map[string]float64{"480p": 0.05 / 0.11, "720p": 1, "1080p": 0.28 / 0.11}
	case constant.ArgolinkSeedance20FastModel:
		return map[string]float64{"480p": 0.04 / 0.091, "720p": 1}
	default:
		return nil
	}
}

func SeedanceVideoInputRatio(model string) float64 {
	if model == constant.ArgolinkSeedance25Model {
		return 1.6
	}
	return 2
}
