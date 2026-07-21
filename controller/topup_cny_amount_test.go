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
	originalWaffoCurrency := setting.WaffoCurrency
	originalWaffoUnitPrice := setting.WaffoUnitPrice
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	originalAmountOptions := operation_setting.GetPaymentSetting().AmountOptions
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountCurrency = originalAmountCurrency
		operation_setting.Price = originalPrice
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.WaffoPancakeUnitPrice = originalPancakeUnitPrice
		setting.WaffoCurrency = originalWaffoCurrency
		setting.WaffoUnitPrice = originalWaffoUnitPrice
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		operation_setting.GetPaymentSetting().AmountOptions = originalAmountOptions
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.GetPaymentSetting().AmountCurrency = operation_setting.TopUpAmountCurrencyCNY
	operation_setting.Price = 7.3
	operation_setting.USDExchangeRate = 7.3
	setting.WaffoPancakeUnitPrice = 1
	setting.WaffoCurrency = operation_setting.TopUpAmountCurrencyCNY
	setting.WaffoUnitPrice = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.GetPaymentSetting().AmountOptions = []int{10, 100, 500, 900}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
}

func TestCNYTopUpAmountsChargeSelectedAmount(t *testing.T) {
	configureCNYTopUpAmountTest(t)

	assert.InDelta(t, 10, getPayMoney(10, "default"), 0.0001)
	assert.InDelta(t, 10, getWaffoPancakePayMoney(10, "default"), 0.0001)
	assert.InDelta(t, 10, getWaffoPayMoney(10, "default"), 0.0001)

	setting.WaffoCurrency = operation_setting.TopUpAmountCurrencyUSD
	assert.InDelta(t, 1.37, getWaffoPayMoney(10, "default"), 0.0001)

	setting.WaffoCurrency = "JPY"
	assert.Zero(t, getWaffoPayMoney(10, "default"))
}

func TestCNYTopUpAmountsRequireConfiguredPreset(t *testing.T) {
	configureCNYTopUpAmountTest(t)

	require.NoError(t, validateTopUpAmount(10, 1))
	require.ErrorContains(t, validateTopUpAmount(11, 1), "固定档位")
	require.ErrorContains(t, validateTopUpAmount(10_001, 1), "不能大于")
}

func TestTokenTopUpAmountUsesMonetaryUpperBound(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	minimum := int64(common.QuotaPerUnit)
	require.NoError(t, validateTopUpAmount(minimum, minimum))
	require.ErrorContains(t, validateTopUpAmount(minimum*(maxOnlineTopUpAmount+1), minimum), "不能大于")
}

func TestCNYTopUpAmountSnapshotsWalletQuota(t *testing.T) {
	configureCNYTopUpAmountTest(t)

	quota, err := calculateTopUpQuotaAmount(10)

	require.NoError(t, err)
	assert.Equal(t, 684932, quota)
}
