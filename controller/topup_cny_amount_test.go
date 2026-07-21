package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureCNYTopUpAmountTest(t *testing.T) {
	t.Helper()

	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalAmountCurrency := operation_setting.GetPaymentSetting().AmountCurrency
	originalPrice := operation_setting.Price
	originalExchangeRate := operation_setting.USDExchangeRate
	originalPancakeUnitPrice := setting.WaffoPancakeUnitPrice
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountCurrency = originalAmountCurrency
		operation_setting.Price = originalPrice
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.WaffoPancakeUnitPrice = originalPancakeUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.GetPaymentSetting().AmountCurrency = operation_setting.TopUpAmountCurrencyCNY
	operation_setting.Price = 7.3
	operation_setting.USDExchangeRate = 7.3
	setting.WaffoPancakeUnitPrice = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
}

func TestCNYTopUpAmountsChargeSelectedAmount(t *testing.T) {
	configureCNYTopUpAmountTest(t)

	assert.InDelta(t, 10, getPayMoney(10, "default"), 0.0001)
	assert.InDelta(t, 10, getWaffoPancakePayMoney(10, "default"), 0.0001)
}

func TestCNYTopUpAmountSnapshotsWalletQuota(t *testing.T) {
	configureCNYTopUpAmountTest(t)

	quota, err := calculateTopUpQuotaAmount(10)

	require.NoError(t, err)
	assert.Equal(t, 684931, quota)
}
