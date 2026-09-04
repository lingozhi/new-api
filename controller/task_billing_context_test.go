package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTaskBillingContextKeepsSubtitleRemovalDurationSettled(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "subtitle-remove",
		PriceData: types.PriceData{
			ModelPrice: 0.02,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}

	billing := buildTaskBillingContext(info)

	require.NotNil(t, billing)
	assert.Equal(t, "subtitle-remove", billing.OriginModelName)
	assert.False(t, billing.PerCallBilling)
}

func TestBuildTaskBillingContextKeepsFixedPriceImagePerCall(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "background-remove",
		PriceData: types.PriceData{
			ModelPrice: 0.05,
			UsePrice:   true,
		},
	}

	billing := buildTaskBillingContext(info)

	require.NotNil(t, billing)
	assert.True(t, billing.PerCallBilling)
}

func TestSeedanceUsesDurationSettlement(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: constant.ArgolinkSeedance25Model}
	info.PriceData.UsePrice = true
	assert.False(t, buildTaskBillingContext(info).PerCallBilling)
}
