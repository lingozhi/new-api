package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSaveWaffoPancakeConfigPersistsEnvironment(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	privateKey := newWaffoPancakeTestPrivateKey(t)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = waffoPancakeCatalogTransport(
		t,
		privateKey,
		setting.WaffoPancakeEnvironmentProd,
		true,
	)

	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalReturnURL := setting.WaffoPancakeReturnURL
	originalStoreID := setting.WaffoPancakeStoreID
	originalProductID := setting.WaffoPancakeProductID
	originalEnvironment := setting.WaffoPancakeEnvironment
	keys := []string{
		"WaffoPancakeMerchantID",
		"WaffoPancakePrivateKey",
		"WaffoPancakeReturnURL",
		"WaffoPancakeStoreID",
		"WaffoPancakeProductID",
		"WaffoPancakeEnvironment",
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	oldOptions := make(map[string]string, len(keys))
	hadOptions := make(map[string]bool, len(keys))
	for _, key := range keys {
		oldOptions[key], hadOptions[key] = common.OptionMap[key]
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeReturnURL = originalReturnURL
		setting.WaffoPancakeStoreID = originalStoreID
		setting.WaffoPancakeProductID = originalProductID
		setting.WaffoPancakeEnvironment = originalEnvironment
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for _, key := range keys {
			if hadOptions[key] {
				common.OptionMap[key] = oldOptions[key]
			} else {
				delete(common.OptionMap, key)
			}
		}
	})

	err := SaveWaffoPancakeConfig(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		privateKey,
		"https://example.com/return",
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"PROD_AbCdEfGhIjKlMnOpQrStUv",
		setting.WaffoPancakeEnvironmentTest,
	)
	require.ErrorContains(t, err, "key environment mismatch")
	var optionCount int64
	require.NoError(t, db.Model(&model.Option{}).
		Where("key = ?", "WaffoPancakeEnvironment").
		Count(&optionCount).Error)
	assert.Zero(t, optionCount)

	err = SaveWaffoPancakeConfig(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		privateKey,
		"https://example.com/return",
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"PROD_AbCdEfGhIjKlMnOpQrStUv",
		setting.WaffoPancakeEnvironmentProd,
	)
	require.NoError(t, err)
	assert.Equal(t, setting.WaffoPancakeEnvironmentProd, setting.WaffoPancakeEnvironment)
	var option model.Option
	require.NoError(t, db.Where("key = ?", "WaffoPancakeEnvironment").First(&option).Error)
	assert.Equal(t, setting.WaffoPancakeEnvironmentProd, option.Value)
}

func TestListWaffoPancakeCatalogDetectsEnvironmentAndValidatesBinding(t *testing.T) {
	privateKey := newWaffoPancakeTestPrivateKey(t)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = waffoPancakeCatalogTransport(
		t,
		privateKey,
		setting.WaffoPancakeEnvironmentProd,
		true,
	)
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })

	catalog, err := ListWaffoPancakeCatalog(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		privateKey,
	)
	require.NoError(t, err)
	assert.Equal(t, setting.WaffoPancakeEnvironmentProd, catalog.Environment)
	require.NoError(t, catalog.ValidateBinding(
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"PROD_AbCdEfGhIjKlMnOpQrStUv",
	))
	require.ErrorContains(t, catalog.ValidateBinding(
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"PROD_inactive",
	), "not active")
}

func TestListWaffoPancakeCatalogRejectsUnapprovedProductionStore(t *testing.T) {
	privateKey := newWaffoPancakeTestPrivateKey(t)
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = waffoPancakeCatalogTransport(
		t,
		privateKey,
		setting.WaffoPancakeEnvironmentProd,
		false,
	)
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })

	catalog, err := ListWaffoPancakeCatalog(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		privateKey,
	)
	require.NoError(t, err)
	require.ErrorContains(t, catalog.ValidateBinding(
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"PROD_AbCdEfGhIjKlMnOpQrStUv",
	), "not approved for production")
}

func waffoPancakeCatalogTransport(t *testing.T, privateKey, environment string, prodEnabled bool) http.RoundTripper {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"data": map[string]any{
			"apiKeys": []map[string]any{{
				"privateKey":  privateKey,
				"environment": environment,
			}},
			"stores": []map[string]any{{
				"id":          "STO_AbCdEfGhIjKlMnOpQrStUv",
				"name":        "Opwan",
				"status":      "active",
				"prodEnabled": prodEnabled,
				"onetimeProducts": []map[string]any{
					{
						"id":     "PROD_AbCdEfGhIjKlMnOpQrStUv",
						"name":   "Opwan API Credits",
						"status": "active",
					},
					{
						"id":     "PROD_inactive",
						"name":   "Inactive",
						"status": "inactive",
					},
				},
			}},
		},
	})
	require.NoError(t, err)
	return waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/graphql", req.URL.Path)
		return waffoPancakeTestResponse(string(payload)), nil
	})
}

func setupWaffoPancakeSettlementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
	})
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionOrder{}))
	return db
}

func validWaffoPancakeSettlementEvent(tradeNo string, userID int) *WaffoPancakeWebhookEvent {
	return &WaffoPancakeWebhookEvent{
		StoreID: "STO_AbCdEfGhIjKlMnOpQrStUv",
		Mode:    setting.WaffoPancakeEnvironmentTest,
		Data: WaffoPancakeWebhookData{
			OrderMerchantExternalID:       tradeNo,
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(userID),
			Currency:                      "USD",
			Amount:                        "10.25",
		},
	}
}

func TestResolveWaffoPancakeTradeNoValidatesSettlementSnapshot(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	const tradeNo = "WAFFO_PANCAKE-101-settlement"
	require.NoError(t, db.Create(&model.TopUp{
		UserId:              101,
		Money:               10.25,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodWaffoPancake,
		PaymentProvider:     model.PaymentProviderWaffoPancake,
		ProviderStoreId:     "STO_AbCdEfGhIjKlMnOpQrStUv",
		ProviderEnvironment: setting.WaffoPancakeEnvironmentTest,
		ProviderCurrency:    "USD",
	}).Error)

	testCases := []struct {
		name        string
		mutate      func(*WaffoPancakeWebhookEvent)
		wantErrText string
	}{
		{name: "valid"},
		{
			name: "cross store",
			mutate: func(event *WaffoPancakeWebhookEvent) {
				event.StoreID = "STO_ZyXwVuTsRqPoNmLkJiHgFe"
			},
			wantErrText: "store mismatch",
		},
		{
			name: "underpay",
			mutate: func(event *WaffoPancakeWebhookEvent) {
				event.Data.Amount = "10.24"
			},
			wantErrText: "amount mismatch",
		},
		{
			name: "currency mismatch",
			mutate: func(event *WaffoPancakeWebhookEvent) {
				event.Data.Currency = "CNY"
			},
			wantErrText: "currency mismatch",
		},
		{
			name: "prod event cannot settle test order",
			mutate: func(event *WaffoPancakeWebhookEvent) {
				event.Mode = setting.WaffoPancakeEnvironmentProd
			},
			wantErrText: "environment mismatch",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := validWaffoPancakeSettlementEvent(tradeNo, 101)
			if tc.mutate != nil {
				tc.mutate(event)
			}
			resolved, err := ResolveWaffoPancakeTradeNo(event)
			if tc.wantErrText == "" {
				require.NoError(t, err)
				assert.Equal(t, tradeNo, resolved)
				return
			}
			require.ErrorContains(t, err, tc.wantErrText)
			assert.Empty(t, resolved)
		})
	}
}

func TestResolveWaffoPancakeTradeNoAllowsLegacyOrderWithoutProviderSnapshot(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	originalStoreID := setting.WaffoPancakeStoreID
	originalEnvironment := setting.WaffoPancakeEnvironment
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	setting.WaffoPancakeStoreID = "STO_AbCdEfGhIjKlMnOpQrStUv"
	setting.WaffoPancakeEnvironment = setting.WaffoPancakeEnvironmentTest
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	t.Cleanup(func() {
		setting.WaffoPancakeStoreID = originalStoreID
		setting.WaffoPancakeEnvironment = originalEnvironment
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
	})
	const tradeNo = "WAFFO_PANCAKE-303-legacy"
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          303,
		Money:           10.25,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
	}).Error)

	event := validWaffoPancakeSettlementEvent(tradeNo, 303)
	event.Data.Currency = "CNY"
	resolved, err := ResolveWaffoPancakeTradeNo(event)
	require.NoError(t, err)
	assert.Equal(t, tradeNo, resolved)

	event.Data.Amount = "10.24"
	_, err = ResolveWaffoPancakeTradeNo(event)
	require.ErrorContains(t, err, "amount mismatch")

	event = validWaffoPancakeSettlementEvent(tradeNo, 303)
	event.Data.Currency = "CNY"
	event.StoreID = "STO_ZyXwVuTsRqPoNmLkJiHgFe"
	_, err = ResolveWaffoPancakeTradeNo(event)
	require.ErrorContains(t, err, "store mismatch")

	event = validWaffoPancakeSettlementEvent(tradeNo, 303)
	event.Data.Currency = "CNY"
	event.Mode = setting.WaffoPancakeEnvironmentProd
	_, err = ResolveWaffoPancakeTradeNo(event)
	require.ErrorContains(t, err, "environment mismatch")

	event = validWaffoPancakeSettlementEvent(tradeNo, 303)
	_, err = ResolveWaffoPancakeTradeNo(event)
	require.ErrorContains(t, err, "currency mismatch")
}

func TestResolveWaffoPancakeTradeNoRejectsLegacyOrderWithoutTrustedBinding(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	originalStoreID := setting.WaffoPancakeStoreID
	originalEnvironment := setting.WaffoPancakeEnvironment
	setting.WaffoPancakeStoreID = ""
	setting.WaffoPancakeEnvironment = ""
	t.Cleanup(func() {
		setting.WaffoPancakeStoreID = originalStoreID
		setting.WaffoPancakeEnvironment = originalEnvironment
	})

	const tradeNo = "WAFFO_PANCAKE-304-unbound"
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          304,
		Money:           10.25,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
	}).Error)

	_, err := ResolveWaffoPancakeTradeNo(validWaffoPancakeSettlementEvent(tradeNo, 304))
	require.ErrorContains(t, err, "trusted store binding")
}

func TestResolveWaffoPancakeSubscriptionTradeNoValidatesSettlementSnapshot(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	const tradeNo = "WAFFO_PANCAKE_SUB-202-settlement"
	require.NoError(t, db.Create(&model.SubscriptionOrder{
		UserId:              202,
		Money:               10.25,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodWaffoPancake,
		PaymentProvider:     model.PaymentProviderWaffoPancake,
		ProviderStoreId:     "STO_AbCdEfGhIjKlMnOpQrStUv",
		ProviderEnvironment: setting.WaffoPancakeEnvironmentTest,
		ProviderCurrency:    "USD",
	}).Error)

	resolved, err := ResolveWaffoPancakeSubscriptionTradeNo(validWaffoPancakeSettlementEvent(tradeNo, 202))
	require.NoError(t, err)
	assert.Equal(t, tradeNo, resolved)
}

func TestResolveWaffoPancakeTradeNoSurfacesDatabaseErrorsForRetry(t *testing.T) {
	db := setupWaffoPancakeSettlementTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = ResolveWaffoPancakeTradeNo(validWaffoPancakeSettlementEvent("WAFFO_PANCAKE-db-error", 1))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWaffoPancakeOrderLookup))
}
