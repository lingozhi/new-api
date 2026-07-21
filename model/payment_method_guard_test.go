package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestRechargeWaffoPancakeUsesSnapshottedQuota(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 102, 0)
	topUp := &TopUp{
		UserId:           102,
		Amount:           10,
		QuotaAmount:      684932,
		Money:            10,
		TradeNo:          "waffo-pancake-cny-snapshot",
		PaymentMethod:    PaymentMethodWaffoPancake,
		PaymentProvider:  PaymentProviderWaffoPancake,
		ProviderCurrency: "CNY",
		Status:           common.TopUpStatusPending,
		CreateTime:       time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, RechargeWaffoPancake(topUp.TradeNo))
	assert.Equal(t, 684932, getUserQuotaForPaymentGuardTest(t, 102))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestUpdatePendingTopUpStatusPreservesDatabaseErrors(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = originalDB })
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.NoError(t, db.Create(&TopUp{
		UserId:          151,
		Amount:          1,
		Money:           1,
		TradeNo:         "waffo-db-error",
		PaymentMethod:   PaymentMethodWaffoPancake,
		PaymentProvider: PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
	}).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	err = UpdatePendingTopUpStatus("waffo-db-error", PaymentProviderWaffoPancake, common.TopUpStatusFailed)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrTopUpNotFound)
	require.ErrorContains(t, err, "closed")
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestUpdatePendingSubscriptionOrderStatusGuardsProviderAndStatus(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 205, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 305)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-status-guard", 205, plan.Id, PaymentProviderWaffoPancake)

	err := UpdatePendingSubscriptionOrderStatus("sub-status-guard", PaymentProviderStripe, common.TopUpStatusFailed)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	order := GetSubscriptionOrderByTradeNo("sub-status-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)

	require.NoError(t, UpdatePendingSubscriptionOrderStatus("sub-status-guard", PaymentProviderWaffoPancake, common.TopUpStatusFailed))
	order = GetSubscriptionOrderByTradeNo("sub-status-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)

	err = UpdatePendingSubscriptionOrderStatus("sub-status-guard", PaymentProviderWaffoPancake, common.TopUpStatusExpired)
	require.ErrorIs(t, err, ErrSubscriptionOrderStatusInvalid)
	order = GetSubscriptionOrderByTradeNo("sub-status-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
}

func TestUpsertSubscriptionTopUp_CopiesProviderSnapshot(t *testing.T) {
	truncateTables(t)

	order := &SubscriptionOrder{
		UserId:              212,
		PlanId:              311,
		Money:               73,
		TradeNo:             "waffo-pancake-subscription-snapshot",
		PaymentMethod:       PaymentMethodWaffoPancake,
		PaymentProvider:     PaymentProviderWaffoPancake,
		ProviderStoreId:     "STO_snapshot",
		ProviderEnvironment: "prod",
		ProviderCurrency:    "CNY",
		Status:              common.TopUpStatusPending,
		CreateTime:          time.Now().Unix(),
	}
	require.NoError(t, upsertSubscriptionTopUpTx(DB, order))

	topUp := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, PaymentProviderWaffoPancake, topUp.PaymentProvider)
	assert.Equal(t, "STO_snapshot", topUp.ProviderStoreId)
	assert.Equal(t, "prod", topUp.ProviderEnvironment)
	assert.Equal(t, "CNY", topUp.ProviderCurrency)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
