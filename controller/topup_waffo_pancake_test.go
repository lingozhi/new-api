package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWaffoPancakeCatalog_UsesOnlyJSONBodyCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalEnvironment := setting.WaffoPancakeEnvironment
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeEnvironment = originalEnvironment
	})
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = ""
	setting.WaffoPancakeEnvironment = setting.WaffoPancakeEnvironmentTest

	testCases := []struct {
		name          string
		target        string
		body          string
		unknownLength bool
		expectedData  string
	}{
		{
			name:         "JSON body credentials are accepted",
			target:       "/api/option/waffo-pancake/catalog",
			body:         `{"merchant_id":"MER_1234567890123456789012","private_key":"not-a-private-key"}`,
			expectedData: "拉取目录失败",
		},
		{
			name:         "query credentials are ignored",
			target:       "/api/option/waffo-pancake/catalog?merchant_id=MER_1234567890123456789012&private_key=not-a-private-key",
			body:         `{}`,
			expectedData: "Waffo Pancake 凭证未配置",
		},
		{
			name:          "chunked JSON body credentials are accepted",
			target:        "/api/option/waffo-pancake/catalog",
			body:          `{"merchant_id":"MER_1234567890123456789012","private_key":"not-a-private-key"}`,
			unknownLength: true,
			expectedData:  "拉取目录失败",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tc.target, bytes.NewBufferString(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			if tc.unknownLength {
				ctx.Request.ContentLength = -1
			}

			ListWaffoPancakeCatalog(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Message string `json:"message"`
				Data    string `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, "error", response.Message)
			assert.Equal(t, tc.expectedData, response.Data)
		})
	}
}

func TestResolveWaffoPancakeAdminCredsFallsBackPerField(t *testing.T) {
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	setting.WaffoPancakeMerchantID = "MER_saved"
	setting.WaffoPancakePrivateKey = "saved-private-key"
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
	})

	merchantID, privateKey := resolveWaffoPancakeAdminCreds("MER_typed", "")
	assert.Equal(t, "MER_typed", merchantID)
	assert.Equal(t, "saved-private-key", privateKey)

	merchantID, privateKey = resolveWaffoPancakeAdminCreds("", "typed-private-key")
	assert.Equal(t, "MER_saved", merchantID)
	assert.Equal(t, "typed-private-key", privateKey)
}

func TestWaffoPancakeWebhookLogsOnlyRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/waffo-pancake/webhook/test?private_key=webhook-secret",
		nil,
	)
	ctx.Params = gin.Params{{Key: "env", Value: setting.WaffoPancakeEnvironmentTest}}

	WaffoPancakeWebhook(ctx)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, logs.String(), `path="/api/waffo-pancake/webhook/test"`)
	assert.NotContains(t, logs.String(), "webhook-secret")
}

func TestRejectLegacyWaffoPancakeCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/option/waffo-pancake/catalog?private_key=must-not-redirect", nil)

	RejectLegacyWaffoPancakeCatalog(ctx)

	require.Equal(t, http.StatusGone, recorder.Code)
	var response struct {
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "error", response.Message)
	assert.Equal(t, "Waffo Pancake 目录接口已改用 POST", response.Data)
}

func TestFormatWaffoPancakeAmount_UsesDisplayPriceString(t *testing.T) {
	testCases := []struct {
		name     string
		amount   float64
		expected string
	}{
		{name: "whole amount", amount: 29, expected: "29.00"},
		{name: "decimal amount", amount: 29.9, expected: "29.90"},
		{name: "round half up to cents", amount: 29.999, expected: "30.00"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, formatWaffoPancakeAmount(tc.amount))
		})
	}
}

func TestValidateWaffoPancakeTopUpAmount(t *testing.T) {
	originalMinTopUp := setting.WaffoPancakeMinTopUp
	setting.WaffoPancakeMinTopUp = 10
	t.Cleanup(func() {
		setting.WaffoPancakeMinTopUp = originalMinTopUp
	})

	testCases := []struct {
		name        string
		amount      int64
		expectedErr string
	}{
		{name: "below minimum", amount: 9, expectedErr: "充值数量不能小于 10"},
		{name: "minimum", amount: 10},
		{name: "maximum", amount: maxOnlineTopUpAmount},
		{name: "above maximum", amount: maxOnlineTopUpAmount + 1, expectedErr: "充值数量不能大于 10000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWaffoPancakeTopUpAmount(tc.amount)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestGetWaffoPancakePayMoney(t *testing.T) {
	originalUnitPrice := setting.WaffoPancakeUnitPrice
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()

	t.Cleanup(func() {
		setting.WaffoPancakeUnitPrice = originalUnitPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.USDExchangeRate = originalUSDExchangeRate
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	setting.WaffoPancakeUnitPrice = 2.5
	operation_setting.USDExchangeRate = 7.3
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{
		10:                           0.8,
		int(common.QuotaPerUnit * 3): 0.5,
		20:                           0,
	}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	testCases := []struct {
		name             string
		amount           int64
		group            string
		quotaDisplayType string
		expected         float64
	}{
		{
			name:             "currency display applies unit price group ratio and discount",
			amount:           10,
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         24,
		},
		{
			name:             "CNY display converts the USD charge with the configured exchange rate",
			amount:           10,
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeCNY,
			expected:         175.2,
		},
		{
			name:             "tokens display converts quota to display units before pricing",
			amount:           int64(common.QuotaPerUnit * 3),
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeTokens,
			expected:         4.5,
		},
		{
			name:             "non-positive discount falls back to no discount",
			amount:           20,
			group:            "default",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.quotaDisplayType
			actual := getWaffoPancakePayMoney(tc.amount, tc.group)
			require.InDelta(t, tc.expected, actual, 0.000001)
		})
	}
}

func TestGetWaffoPancakeCheckoutAmount(t *testing.T) {
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.USDExchangeRate = originalUSDExchangeRate
	})

	operation_setting.USDExchangeRate = 7.3

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	require.InDelta(t, 73, getWaffoPancakeCheckoutAmount(10), 0.000001)

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.InDelta(t, 10, getWaffoPancakeCheckoutAmount(10), 0.000001)
	require.InDelta(t, 10.01, getWaffoPancakeCheckoutAmount(10.005), 0.000001)
}

func TestGetWaffoPancakeCheckoutCurrency(t *testing.T) {
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
	})

	testCases := []struct {
		name             string
		quotaDisplayType string
		expected         string
	}{
		{name: "CNY display", quotaDisplayType: operation_setting.QuotaDisplayTypeCNY, expected: "CNY"},
		{name: "USD display", quotaDisplayType: operation_setting.QuotaDisplayTypeUSD, expected: "USD"},
		{name: "token display", quotaDisplayType: operation_setting.QuotaDisplayTypeTokens, expected: "USD"},
		{name: "unknown display", quotaDisplayType: "UNKNOWN", expected: "USD"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.quotaDisplayType
			require.Equal(t, tc.expected, getWaffoPancakeCheckoutCurrency())
		})
	}
}
