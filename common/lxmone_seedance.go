package common

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

func LxmoneSeedanceModel(name string) string {
	switch name {
	case "seedance-2", "seedance-2-pro":
		return "seedance-2-pro"
	case "seedance-2.5", "seedance-2.5-pro":
		return "seedance-2.5-pro"
	case "seedance-2-fast", "seedance-2-mini":
		return name
	default:
		return ""
	}
}

func IsLxmoneSeedance(baseURL, model string) bool {
	base, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(base.Hostname(), "lxmone.xyz") && LxmoneSeedanceModel(model) != ""
}

// SeedanceRequestPathSupported keeps incompatible providers out of both cached
// and database-backed selection. Unrelated models retain existing routing.
func SeedanceRequestPathSupported(baseURL, model, path string) bool {
	if IsLxmoneSeedance(baseURL, model) {
		return path == "/v1/videos" || (strings.HasPrefix(path, "/v1/videos/") && !strings.HasSuffix(path, "/remix"))
	}
	switch strings.ToLower(strings.TrimSpace(model)) {
	case constant.ArgolinkSeedance25Model, constant.ArgolinkSeedance20Model, constant.ArgolinkSeedance20FastModel:
		return path == "/v1/media/uploads" || path == "/v1/videos/generations" || (strings.HasPrefix(path, "/v1/videos/") && !strings.HasSuffix(path, "/remix"))
	default:
		return true
	}
}
