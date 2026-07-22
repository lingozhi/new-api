package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveTopupGroupRatio(t *testing.T) {
	t.Helper()
	original := TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateTopupGroupRatioByJSONString(original))
	})
}

func TestUpdateTopupGroupRatioAddsDefaultWhenConfigurationOmitsIt(t *testing.T) {
	preserveTopupGroupRatio(t)

	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"gpt pro":1,"gpt plus":1}`))
	assert.Equal(t, 1.0, GetTopupGroupRatio("default"))

	var ratios map[string]float64
	require.NoError(t, UnmarshalJsonStr(TopupGroupRatio2JSONString(), &ratios))
	assert.Equal(t, 1.0, ratios["default"], "the normalized option must explicitly preserve the default price")
}

func TestUpdateTopupGroupRatioPreservesExplicitDefault(t *testing.T) {
	preserveTopupGroupRatio(t)

	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"default":0.8,"vip":1.2}`))
	assert.Equal(t, 0.8, GetTopupGroupRatio("default"))
}

func TestUpdateTopupGroupRatioKeepsPreviousConfigurationOnInvalidJSON(t *testing.T) {
	preserveTopupGroupRatio(t)
	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"default":0.75,"vip":1.2}`))

	require.Error(t, UpdateTopupGroupRatioByJSONString(`{"default":`))
	assert.Equal(t, 0.75, GetTopupGroupRatio("default"))
	assert.Equal(t, 1.2, GetTopupGroupRatio("vip"))
}
