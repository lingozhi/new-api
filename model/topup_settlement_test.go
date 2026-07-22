package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTopUpSettlement(t *testing.T) {
	topUp := &TopUp{
		Money:            10,
		TradeNo:          "WAFFO-SETTLEMENT-1",
		ProviderCurrency: "CNY",
	}

	require.NoError(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "10.00",
		Currency:         "cny",
		PaymentRequestID: topUp.TradeNo,
	}, true))
	require.ErrorContains(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "9.99",
		Currency:         "CNY",
		PaymentRequestID: topUp.TradeNo,
	}, true), "金额不匹配")
	require.ErrorContains(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "10.00",
		Currency:         "USD",
		PaymentRequestID: topUp.TradeNo,
	}, true), "币种不匹配")
	require.ErrorContains(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "10.00",
		Currency:         "CNY",
		PaymentRequestID: "other",
	}, true), "标识不匹配")
}

func TestValidateTopUpSettlementAcceptsSignedLegacyCurrency(t *testing.T) {
	topUp := &TopUp{Money: 10, TradeNo: "WAFFO-LEGACY-1"}

	require.NoError(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "10.00",
		Currency:         "USD",
		PaymentRequestID: topUp.TradeNo,
	}, true))
	require.ErrorContains(t, validateTopUpSettlement(topUp, TopUpSettlement{
		Amount:           "10.00",
		Currency:         "not-a-currency",
		PaymentRequestID: topUp.TradeNo,
	}, true), "币种无效")
}
