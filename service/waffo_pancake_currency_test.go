package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

type waffoPancakeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f waffoPancakeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newWaffoPancakeTestPrivateKey(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
}

func waffoPancakeTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCreateWaffoPancakeCheckoutSession_UsesRequestedCurrency(t *testing.T) {
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		http.DefaultClient.Transport = originalTransport
	})

	setting.WaffoPancakeMerchantID = "MER_AbCdEfGhIjKlMnOpQrStUv"
	setting.WaffoPancakePrivateKey = newWaffoPancakeTestPrivateKey(t)

	var mu sync.Mutex
	checkoutCurrencies := make([]string, 0, 2)
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			return waffoPancakeTestResponse(`{"data":{"token":"JWT","expiresAt":"2026-07-21T01:00:00Z"}}`), nil
		case "/v1/actions/checkout/create-session":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var payload struct {
				Currency string `json:"currency"`
			}
			require.NoError(t, common.Unmarshal(body, &payload))
			mu.Lock()
			checkoutCurrencies = append(checkoutCurrencies, payload.Currency)
			mu.Unlock()
			return waffoPancakeTestResponse(`{"data":{"sessionId":"ses_1","checkoutUrl":"https://pancake.example/checkout/abc","expiresAt":"2026-07-21T00:45:00Z"}}`), nil
		default:
			t.Fatalf("unexpected Waffo Pancake path: %s", req.URL.Path)
			return nil, nil
		}
	})

	testCases := []struct {
		name     string
		currency string
		expected string
	}{
		{name: "CNY checkout", currency: "CNY", expected: "CNY"},
		{name: "blank currency remains backwards compatible", currency: "", expected: "USD"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateWaffoPancakeCheckoutSession(context.Background(), &WaffoPancakeCreateSessionParams{
				ProductID:               "PROD_AbCdEfGhIjKlMnOpQrStUv",
				Currency:                tc.currency,
				BuyerIdentity:           "new-api-user-1",
				OrderMerchantExternalID: "WAFFO_PANCAKE-1-test",
			})
			require.NoError(t, err)
		})
	}

	require.Equal(t, []string{"CNY", "USD"}, checkoutCurrencies)
}

func TestCreateWaffoPancakePrimaryProduct_CreatesUSDAndCNYPrices(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		operation_setting.USDExchangeRate = originalUSDExchangeRate
	})
	operation_setting.USDExchangeRate = 7.3

	var createPayload struct {
		Prices map[string]struct {
			Amount      string `json:"amount"`
			TaxCategory string `json:"taxCategory"`
		} `json:"prices"`
	}
	publishCalls := 0
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/onetime-product/create-product":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(body, &createPayload))
			return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"1.00","taxCategory":"saas"}},"status":"active"}}}`), nil
		case "/v1/graphql":
			return waffoPancakeTestResponse(`{"data":{"onetimeProductVersions":[{"isProdVersion":false}]}}`), nil
		case "/v1/actions/onetime-product/publish-product":
			publishCalls++
			return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"1.00","taxCategory":"saas"}},"status":"active"}}}`), nil
		default:
			t.Fatalf("unexpected Waffo Pancake path: %s", req.URL.Path)
			return nil, nil
		}
	})

	productID, err := CreateWaffoPancakePrimaryProduct(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		newWaffoPancakeTestPrivateKey(t),
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"https://api.opwan.ai/wallet",
	)
	require.NoError(t, err)
	require.Equal(t, "PROD_AbCdEfGhIjKlMnOpQrStUv", productID)
	require.Equal(t, "1.00", createPayload.Prices["USD"].Amount)
	require.Equal(t, "saas", createPayload.Prices["USD"].TaxCategory)
	require.Equal(t, "7.30", createPayload.Prices["CNY"].Amount)
	require.Equal(t, "saas", createPayload.Prices["CNY"].TaxCategory)
	require.Equal(t, 1, publishCalls)
}

func TestCreateWaffoPancakePrimaryProduct_SkipsPublishForProductionVersion(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	publishCalls := 0
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/onetime-product/create-product":
			return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"1.00","taxCategory":"saas"}},"status":"active"}}}`), nil
		case "/v1/graphql":
			return waffoPancakeTestResponse(`{"data":{"onetimeProductVersions":[{"isProdVersion":true}]}}`), nil
		case "/v1/actions/onetime-product/publish-product":
			publishCalls++
			return waffoPancakeTestResponse(`{"data":null,"errors":[{"message":"No test version found","layer":"gateway"}]}`), nil
		default:
			t.Fatalf("unexpected Waffo Pancake path: %s", req.URL.Path)
			return nil, nil
		}
	})

	productID, err := CreateWaffoPancakePrimaryProduct(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		newWaffoPancakeTestPrivateKey(t),
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"https://api.opwan.ai/wallet",
	)
	require.NoError(t, err)
	require.Equal(t, "PROD_AbCdEfGhIjKlMnOpQrStUv", productID)
	require.Zero(t, publishCalls)
}
