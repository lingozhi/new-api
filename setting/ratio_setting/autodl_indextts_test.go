package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexTTS2DefaultPriceMatchesAutoDLPerTaskContract(t *testing.T) {
	prices := GetDefaultModelPriceMap()
	price, exists := prices[constant.AutoDLModelIndexTTS2]
	require.True(t, exists)
	assert.InDelta(t, 0.01/USD2RMB, price, 1e-12)
}
