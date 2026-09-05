package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceResolutionPricesFollowConfiguredBase(t *testing.T) {
	for _, tc := range []struct {
		model      string
		base       float64
		prices     map[string]float64
		videoRatio float64
	}{
		{"seedance-2.0", 0.803, map[string]float64{"480p": 0.365, "720p": 0.803, "1080p": 2.044}, 2},
		{"seedance-2.0-fast", 0.6643, map[string]float64{"480p": 0.292, "720p": 0.6643}, 2},
		{"seedance-2.5", 1.241, map[string]float64{"480p": 0.5621, "720p": 1.241, "1080p": 3.139}, 1.6},
	} {
		t.Run(tc.model, func(t *testing.T) {
			ratios := SeedanceResolutionRatios(tc.model)
			require.Len(t, ratios, len(tc.prices))
			for resolution, price := range tc.prices {
				assert.InDelta(t, price, tc.base*ratios[resolution], 1e-10)
			}
			assert.Equal(t, tc.videoRatio, SeedanceVideoInputRatio(tc.model))
		})
	}
	assert.Nil(t, SeedanceResolutionRatios("gpt-image-2"))
}
