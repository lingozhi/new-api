package middleware

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSeedanceSharedNameUsesProviderEndpoint(t *testing.T) {
	for _, tc := range []struct {
		base, path string
		want       bool
	}{
		{"https://lxmone.xyz", "/v1/videos", true},
		{"https://lxmone.xyz", "/v1/videos/generations", false},
		{"https://argolink.io", "/v1/videos", false},
		{"https://argolink.io", "/v1/videos/generations", true},
		{"https://lxmone.xyz", "/v1/videos/test/remix", false},
		{"https://lxmone.xyz", "/v1/videos/test", true},
		{"https://lxmone.xyz", "/v1/videos/test/content", true},
	} {
		ch := &model.Channel{Type: constant.ChannelTypeOpenAI, BaseURL: &tc.base}
		assert.Equal(t, tc.want, channelSupportsRequestPath(ch, tc.path, "seedance-2.5"), tc.base+tc.path)
	}
}
