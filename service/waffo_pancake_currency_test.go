package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
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

func TestPostWaffoPancakeActionSignsRequest(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))

	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "MER_AbCdEfGhIjKlMnOpQrStUv", req.Header.Get("X-Merchant-Id"))
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		require.Len(t, req.Header.Get("X-Idempotency-Key"), sha256.Size*2)

		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		timestamp := req.Header.Get("X-Timestamp")
		_, err = strconv.ParseInt(timestamp, 10, 64)
		require.NoError(t, err)
		bodyHash := sha256.Sum256(body)
		canonical := req.Method + "\n" + req.URL.Path + "\n" + timestamp + "\n" + base64.StdEncoding.EncodeToString(bodyHash[:])
		canonicalHash := sha256.Sum256([]byte(canonical))
		signature, err := base64.StdEncoding.DecodeString(req.Header.Get("X-Signature"))
		require.NoError(t, err)
		require.NoError(t, rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, canonicalHash[:], signature))
		return waffoPancakeTestResponse(`{"data":{"ok":true}}`), nil
	})

	result, err := postWaffoPancakeAction[struct {
		OK bool `json:"ok"`
	}](context.Background(), "MER_AbCdEfGhIjKlMnOpQrStUv", privateKeyPEM, "/v1/actions/test", map[string]string{"value": "signed"}, 60)
	require.NoError(t, err)
	require.True(t, result.OK)
}

func TestPostWaffoPancakeActionClassifiesFailuresAndPreservesErrors(t *testing.T) {
	privateKey := newWaffoPancakeTestPrivateKey(t)
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })

	testCases := []struct {
		name          string
		transport     waffoPancakeRoundTripFunc
		ambiguous     bool
		errorContains []string
	}{
		{
			name: "transport failure is ambiguous",
			transport: func(_ *http.Request) (*http.Response, error) {
				return nil, io.ErrUnexpectedEOF
			},
			ambiguous:     true,
			errorContains: []string{"unexpected EOF"},
		},
		{
			name: "unprocessable API rejection is conclusive and keeps every notice",
			transport: func(_ *http.Request) (*http.Response, error) {
				response := waffoPancakeTestResponse(`{"data":{"ok":"wrong type"},"errors":[{"message":"bad price","layer":"product","aiHint":"use CNY"},{"message":"checkout rejected","layer":"gateway"}]}`)
				response.StatusCode = http.StatusUnprocessableEntity
				return response, nil
			},
			errorContains: []string{"bad price", "layer=product", "hint=use CNY", "checkout rejected", "layer=gateway"},
		},
		{
			name: "server error remains ambiguous",
			transport: func(_ *http.Request) (*http.Response, error) {
				response := waffoPancakeTestResponse(`{"data":null,"errors":[{"message":"upstream unavailable","layer":"gateway"}]}`)
				response.StatusCode = http.StatusBadGateway
				return response, nil
			},
			ambiguous:     true,
			errorContains: []string{"upstream unavailable"},
		},
		{
			name: "malformed success data remains ambiguous",
			transport: func(_ *http.Request) (*http.Response, error) {
				return waffoPancakeTestResponse(`{"data":{"ok":"wrong type"}}`), nil
			},
			ambiguous:     true,
			errorContains: []string{"decode response data"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultClient.Transport = tc.transport
			_, err := postWaffoPancakeAction[struct {
				OK bool `json:"ok"`
			}](context.Background(), "MER_AbCdEfGhIjKlMnOpQrStUv", privateKey, "/v1/actions/test", map[string]string{"value": "test"}, 60)
			require.Error(t, err)
			require.Equal(t, tc.ambiguous, IsWaffoPancakeActionOutcomeAmbiguous(err))
			for _, expected := range tc.errorContains {
				require.ErrorContains(t, err, expected)
			}
		})
	}
}

func TestPostWaffoPancakeActionLogsWarnings(t *testing.T) {
	privateKey := newWaffoPancakeTestPrivateKey(t)
	originalTransport := http.DefaultClient.Transport
	originalWriter := gin.DefaultWriter
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		common.LogWriterMu.Unlock()
	})

	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return waffoPancakeTestResponse(`{"data":{"ok":true},"warnings":[{"message":"tax fallback","layer":"product","aiHint":"review category"}]}`), nil
	})
	result, err := postWaffoPancakeAction[struct {
		OK bool `json:"ok"`
	}](context.Background(), "MER_AbCdEfGhIjKlMnOpQrStUv", privateKey, "/v1/actions/test", map[string]string{"value": "test"}, 60)
	require.NoError(t, err)
	require.True(t, result.OK)
	require.Contains(t, logs.String(), "tax fallback")
	require.Contains(t, logs.String(), "layer=product")
	require.Contains(t, logs.String(), "hint=review category")
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
	type checkoutCapture struct {
		Currency      string `json:"currency"`
		PriceSnapshot struct {
			TaxIncluded bool `json:"taxIncluded"`
		} `json:"priceSnapshot"`
	}
	checkoutPayloads := make([]checkoutCapture, 0, 2)
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			return waffoPancakeTestResponse(`{"data":{"token":"JWT","expiresAt":"2026-07-21T01:00:00Z"}}`), nil
		case "/v1/actions/checkout/create-session":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var payload checkoutCapture
			require.NoError(t, common.Unmarshal(body, &payload))
			mu.Lock()
			checkoutPayloads = append(checkoutPayloads, payload)
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
				PriceSnapshot: &WaffoPancakePriceSnapshot{
					Amount:      "1.00",
					TaxCategory: "saas",
				},
			})
			require.NoError(t, err)
		})
	}

	require.Equal(t, "CNY", checkoutPayloads[0].Currency)
	require.True(t, checkoutPayloads[0].PriceSnapshot.TaxIncluded)
	require.Equal(t, "USD", checkoutPayloads[1].Currency)
	require.True(t, checkoutPayloads[1].PriceSnapshot.TaxIncluded)
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
			TaxIncluded bool   `json:"taxIncluded"`
			TaxCategory string `json:"taxCategory"`
		} `json:"prices"`
	}
	publishCalls := 0
	versionQueries := 0
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/onetime-product/create-product":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(body, &createPayload))
			return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"1.00","taxCategory":"saas"}},"status":"active"}}}`), nil
		case "/v1/graphql":
			versionQueries++
			if versionQueries == 1 {
				return waffoPancakeTestResponse(`{"data":{"onetimeProductVersions":[{"isProdVersion":false}]}}`), nil
			}
			return waffoPancakeTestResponse(`{"data":{"onetimeProductVersions":[{"isProdVersion":true}]}}`), nil
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
		setting.WaffoPancakeEnvironmentProd,
	)
	require.NoError(t, err)
	require.Equal(t, "PROD_AbCdEfGhIjKlMnOpQrStUv", productID)
	require.Equal(t, "1.00", createPayload.Prices["USD"].Amount)
	require.True(t, createPayload.Prices["USD"].TaxIncluded)
	require.Equal(t, "saas", createPayload.Prices["USD"].TaxCategory)
	require.Equal(t, "7.30", createPayload.Prices["CNY"].Amount)
	require.True(t, createPayload.Prices["CNY"].TaxIncluded)
	require.Equal(t, "saas", createPayload.Prices["CNY"].TaxCategory)
	require.Equal(t, 1, publishCalls)
	require.Equal(t, 2, versionQueries)
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
		setting.WaffoPancakeEnvironmentProd,
	)
	require.NoError(t, err)
	require.Equal(t, "PROD_AbCdEfGhIjKlMnOpQrStUv", productID)
	require.Zero(t, publishCalls)
}

func TestCreateWaffoPancakePrimaryProduct_RejectsEmptyPublishResult(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })

	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/actions/onetime-product/create-product":
			return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"7.30","taxCategory":"saas"}},"status":"active"}}}`), nil
		case "/v1/graphql":
			return waffoPancakeTestResponse(`{"data":{"onetimeProductVersions":[{"isProdVersion":false}]}}`), nil
		case "/v1/actions/onetime-product/publish-product":
			return waffoPancakeTestResponse(`{"data":null}`), nil
		default:
			return nil, errors.New("unexpected Waffo Pancake path: " + req.URL.Path)
		}
	})

	_, err := CreateWaffoPancakePrimaryProduct(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		newWaffoPancakeTestPrivateKey(t),
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"https://api.opwan.ai/wallet",
		setting.WaffoPancakeEnvironmentProd,
	)
	require.ErrorContains(t, err, "publish returned an invalid product")
}

func TestCreateWaffoPancakePrimaryProduct_TestEnvironmentDoesNotPublish(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/actions/onetime-product/create-product", req.URL.Path)
		return waffoPancakeTestResponse(`{"data":{"product":{"id":"PROD_AbCdEfGhIjKlMnOpQrStUv","storeId":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-charge-product","prices":{"USD":{"amount":"1.00","taxCategory":"saas"},"CNY":{"amount":"7.30","taxCategory":"saas"}},"status":"active"}}}`), nil
	})

	productID, err := CreateWaffoPancakePrimaryProduct(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		newWaffoPancakeTestPrivateKey(t),
		"STO_AbCdEfGhIjKlMnOpQrStUv",
		"https://api.opwan.ai/wallet",
		setting.WaffoPancakeEnvironmentTest,
	)
	require.NoError(t, err)
	require.Equal(t, "PROD_AbCdEfGhIjKlMnOpQrStUv", productID)
}

func TestWaffoPancakeClientAppliesRequestTimeout(t *testing.T) {
	originalTransport := http.DefaultClient.Transport
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	deadlineRemaining := make(chan time.Duration, 1)
	http.DefaultClient.Transport = waffoPancakeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		deadlineRemaining <- time.Until(deadline)
		return waffoPancakeTestResponse(`{"data":{"store":{"id":"STO_AbCdEfGhIjKlMnOpQrStUv","name":"new-api-store"}}}`), nil
	})

	storeID, err := CreateWaffoPancakePrimaryStore(
		context.Background(),
		"MER_AbCdEfGhIjKlMnOpQrStUv",
		newWaffoPancakeTestPrivateKey(t),
	)
	require.NoError(t, err)
	require.Equal(t, "STO_AbCdEfGhIjKlMnOpQrStUv", storeID)
	require.InDelta(t, waffoPancakeHTTPTimeout.Seconds(), (<-deadlineRemaining).Seconds(), 1)
}
