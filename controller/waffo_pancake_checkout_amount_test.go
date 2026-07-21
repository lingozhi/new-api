package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type controllerWaffoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f controllerWaffoRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type controllerWaffoCheckoutPayload struct {
	ProductID     string `json:"productId"`
	Currency      string `json:"currency"`
	PriceSnapshot struct {
		Amount      string `json:"amount"`
		TaxCategory string `json:"taxCategory"`
	} `json:"priceSnapshot"`
}

func configureWaffoPancakeCNYCheckoutTest(t *testing.T) {
	t.Helper()
	confirmPaymentComplianceForTest(t)

	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for amount, discount := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[amount] = discount
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	originalUnitPrice := setting.WaffoPancakeUnitPrice
	originalMinTopUp := setting.WaffoPancakeMinTopUp
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.USDExchangeRate = originalUSDExchangeRate
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
		setting.WaffoPancakeUnitPrice = originalUnitPrice
		setting.WaffoPancakeMinTopUp = originalMinTopUp
	})

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encodedPrivateKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.USDExchangeRate = 7.3
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	setting.WaffoPancakeMerchantID = "MER_AbCdEfGhIjKlMnOpQrStUv"
	setting.WaffoPancakePrivateKey = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: encodedPrivateKey,
	}))
	setting.WaffoPancakeProductID = "PROD_AbCdEfGhIjKlMnOpQrStUv"
	setting.WaffoPancakeUnitPrice = 1
	setting.WaffoPancakeMinTopUp = 1
}

func installWaffoPancakeCheckoutCapture(t *testing.T) <-chan controllerWaffoCheckoutPayload {
	t.Helper()

	originalTransport := http.DefaultClient.Transport
	captured := make(chan controllerWaffoCheckoutPayload, 1)
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	http.DefaultClient.Transport = controllerWaffoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			return controllerWaffoTestResponse(`{"data":{"token":"JWT","expiresAt":"2026-07-21T01:00:00Z"}}`), nil
		case "/v1/actions/checkout/create-session":
			var payload controllerWaffoCheckoutPayload
			if err := common.DecodeJson(req.Body, &payload); err != nil {
				return nil, err
			}
			captured <- payload
			return controllerWaffoTestResponse(`{"data":{"sessionId":"ses_1","checkoutUrl":"https://pancake.example/checkout/abc","expiresAt":"2026-07-21T00:45:00Z"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected Waffo Pancake path: %s", req.URL.Path)
		}
	})

	return captured
}

func controllerWaffoTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func requireSuccessfulWaffoPancakeResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "success", response.Message, recorder.Body.String())
}

func TestRequestWaffoPancakePay_ConvertsCNYCheckoutAmount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	require.NoError(t, db.Create(&model.User{
		Id:       101,
		Username: "waffo-wallet",
		Password: "password123",
		Group:    "default",
		Email:    "wallet@example.com",
	}).Error)
	configureWaffoPancakeCNYCheckoutTest(t)
	captured := installWaffoPancakeCheckoutCapture(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 101)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/waffo-pancake/pay",
		strings.NewReader(`{"amount":10}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestWaffoPancakePay(ctx)
	requireSuccessfulWaffoPancakeResponse(t, recorder)

	payload := <-captured
	require.Equal(t, setting.WaffoPancakeProductID, payload.ProductID)
	require.Equal(t, "CNY", payload.Currency)
	require.Equal(t, "73.00", payload.PriceSnapshot.Amount)
	require.Equal(t, "saas", payload.PriceSnapshot.TaxCategory)

	var topUp model.TopUp
	require.NoError(t, db.Where("user_id = ?", 101).First(&topUp).Error)
	require.Equal(t, int64(10), topUp.Amount)
	require.InDelta(t, 73, topUp.Money, 0.000001)
}

func TestSubscriptionRequestWaffoPancakePay_ConvertsCNYCheckoutAmount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionOrder{}))
	require.NoError(t, db.Create(&model.User{
		Id:       102,
		Username: "waffo-plan",
		Password: "password123",
		Group:    "default",
		Email:    "plan@example.com",
	}).Error)
	plan := model.SubscriptionPlan{
		Id:                    201,
		Title:                 "Pro",
		PriceAmount:           10,
		Currency:              "USD",
		DurationUnit:          model.SubscriptionDurationMonth,
		DurationValue:         1,
		Enabled:               true,
		WaffoPancakeProductId: "PROD_ZyXwVuTsRqPoNmLkJiHgFe",
	}
	model.InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() {
		model.InvalidateSubscriptionPlanCache(plan.Id)
	})
	require.NoError(t, db.Create(&plan).Error)
	configureWaffoPancakeCNYCheckoutTest(t)
	captured := installWaffoPancakeCheckoutCapture(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 102)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/subscription/waffo-pancake/pay",
		strings.NewReader(`{"plan_id":201}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	SubscriptionRequestWaffoPancakePay(ctx)
	requireSuccessfulWaffoPancakeResponse(t, recorder)

	payload := <-captured
	require.Equal(t, plan.WaffoPancakeProductId, payload.ProductID)
	require.Equal(t, "CNY", payload.Currency)
	require.Equal(t, "73.00", payload.PriceSnapshot.Amount)
	require.Equal(t, "saas", payload.PriceSnapshot.TaxCategory)

	var order model.SubscriptionOrder
	require.NoError(t, db.Where("user_id = ? AND plan_id = ?", 102, plan.Id).First(&order).Error)
	require.InDelta(t, 73, order.Money, 0.000001)
}
