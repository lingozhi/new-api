package common

// WanVideoResolutionRatios are relative to the configured 720p per-second price.
// The gateway and public price catalog share the same resolution multipliers.
func WanVideoResolutionRatios(model string) map[string]float64 {
	switch model {
	case "wan3.0", "wan3.0-video":
		return map[string]float64{"480p": 0.25 / 0.30, "720p": 1, "1080p": 0.35 / 0.30}
	default:
		return nil
	}
}
