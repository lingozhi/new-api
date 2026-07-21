package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

const waffoPancakeHTTPTimeout = 20 * time.Second

var ErrWaffoPancakeOrderLookup = errors.New("Waffo Pancake order lookup failed")

// WaffoPancakePriceSnapshot is the per-session price override sent with checkout.
type WaffoPancakePriceSnapshot struct {
	Amount      string
	TaxCategory string
}

// WaffoPancakeCreateSessionParams is the input to CreateWaffoPancakeCheckoutSession.
// BuyerIdentity must be stable per user (see WaffoPancakeBuyerIdentityFromUserID).
// OrderMerchantExternalID = our trade_no; Pancake echoes it back in webhooks.
type WaffoPancakeCreateSessionParams struct {
	ProductID               string
	Currency                string
	BuyerIdentity           string
	PriceSnapshot           *WaffoPancakePriceSnapshot
	BuyerEmail              string
	ExpiresInSeconds        *int
	OrderMerchantExternalID string
}

// WaffoPancakeCheckoutSession is the response of CreateWaffoPancakeCheckoutSession.
// CheckoutURL already carries the `#token=...` fragment; Token / TokenExpiresAt
// are exposed separately for self-service flows driven from new-api's own UI.
type WaffoPancakeCheckoutSession struct {
	SessionID      string
	CheckoutURL    string
	ExpiresAt      string
	OrderID        string
	Token          string
	TokenExpiresAt string
}

// WaffoPancakeWebhookEvent mirrors the SDK's WebhookEvent shape using plain
// strings so controllers don't have to import the SDK package.
type WaffoPancakeWebhookEvent struct {
	ID        string
	Timestamp string
	EventType string
	EventID   string
	StoreID   string
	Mode      string
	Data      WaffoPancakeWebhookData
}

type WaffoPancakeWebhookData struct {
	// OrderID = Pancake ORD_* (logs); OrderMerchantExternalID = our trade_no (lookup).
	OrderID                       string
	OrderMerchantExternalID       string
	BuyerEmail                    string
	Currency                      string
	Amount                        string
	TaxAmount                     string
	ProductName                   string
	MerchantProvidedBuyerIdentity string
}

// NormalizedEventType returns the event type or empty string for a nil event.
func (e *WaffoPancakeWebhookEvent) NormalizedEventType() string {
	if e == nil {
		return ""
	}
	return e.EventType
}

func newWaffoPancakeClientFromCreds(merchantID, privateKey string) (*pancake.Client, error) {
	if strings.TrimSpace(merchantID) == "" || strings.TrimSpace(privateKey) == "" {
		return nil, fmt.Errorf("merchant id and private key are required")
	}
	httpClient := *http.DefaultClient
	httpClient.Timeout = waffoPancakeHTTPTimeout
	return pancake.New(pancake.Config{
		MerchantID: merchantID,
		PrivateKey: privateKey,
		HTTPClient: &httpClient,
	})
}

// CreateWaffoPancakeCheckoutSession creates an Authenticated-mode checkout
// session: the order is bound to BuyerIdentity (stable per user) so it stays
// attributable even if the buyer edits the email on Waffo's checkout form.
func CreateWaffoPancakeCheckoutSession(ctx context.Context, params *WaffoPancakeCreateSessionParams) (*WaffoPancakeCheckoutSession, error) {
	if params == nil {
		return nil, fmt.Errorf("missing checkout params")
	}
	if strings.TrimSpace(params.BuyerIdentity) == "" {
		return nil, fmt.Errorf("missing buyer identity")
	}
	if strings.TrimSpace(params.OrderMerchantExternalID) == "" {
		return nil, fmt.Errorf("missing order merchant external id")
	}
	if len(params.OrderMerchantExternalID) > 128 {
		return nil, fmt.Errorf("order merchant external id must be at most 128 characters")
	}
	productID := strings.TrimSpace(params.ProductID)
	if productID == "" {
		return nil, fmt.Errorf("missing product id")
	}
	if params.ExpiresInSeconds != nil && *params.ExpiresInSeconds <= 0 {
		return nil, fmt.Errorf("expires in seconds must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	if currency == "" {
		currency = "USD"
	}
	if len(currency) != 3 {
		return nil, fmt.Errorf("currency must be a three-letter ISO 4217 code")
	}
	if params.PriceSnapshot != nil {
		amount, err := decimal.NewFromString(strings.TrimSpace(params.PriceSnapshot.Amount))
		if err != nil || !amount.IsPositive() {
			return nil, fmt.Errorf("invalid checkout price snapshot amount: %q", params.PriceSnapshot.Amount)
		}
		if strings.TrimSpace(params.PriceSnapshot.TaxCategory) == "" {
			return nil, fmt.Errorf("missing checkout price snapshot tax category")
		}
	}
	merchantID := strings.TrimSpace(setting.WaffoPancakeMerchantID)
	privateKey := strings.TrimSpace(setting.WaffoPancakePrivateKey)
	if merchantID == "" || privateKey == "" {
		return nil, fmt.Errorf("build Waffo Pancake client: merchant id and private key are required")
	}

	token, err := postWaffoPancakeAction[struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	}](ctx, merchantID, privateKey, "/v1/actions/auth/issue-session-token", struct {
		ProductID     string `json:"productId"`
		BuyerIdentity string `json:"buyerIdentity"`
	}{
		ProductID:     productID,
		BuyerIdentity: strings.TrimSpace(params.BuyerIdentity),
	}, 60)
	if err != nil {
		return nil, &waffoPancakeActionError{Cause: fmt.Errorf("issue session token: %w", err)}
	}
	if token == nil || strings.TrimSpace(token.Token) == "" {
		return nil, fmt.Errorf("Waffo Pancake returned an empty session token")
	}

	checkoutRequest := struct {
		ProductID               string                 `json:"productId"`
		Currency                string                 `json:"currency"`
		PriceSnapshot           *waffoPancakePriceInfo `json:"priceSnapshot,omitempty"`
		BuyerEmail              *string                `json:"buyerEmail,omitempty"`
		ExpiresInSeconds        *int                   `json:"expiresInSeconds,omitempty"`
		OrderMerchantExternalID *string                `json:"orderMerchantExternalId,omitempty"`
	}{
		ProductID:               productID,
		Currency:                currency,
		BuyerEmail:              optionalString(params.BuyerEmail),
		ExpiresInSeconds:        params.ExpiresInSeconds,
		OrderMerchantExternalID: optionalString(params.OrderMerchantExternalID),
	}
	if params.PriceSnapshot != nil {
		checkoutRequest.PriceSnapshot = &waffoPancakePriceInfo{
			Amount:      params.PriceSnapshot.Amount,
			TaxIncluded: true,
			TaxCategory: params.PriceSnapshot.TaxCategory,
		}
	}
	session, err := postWaffoPancakeAction[struct {
		SessionID   string `json:"sessionId"`
		CheckoutURL string `json:"checkoutUrl"`
		ExpiresAt   string `json:"expiresAt"`
	}](ctx, merchantID, privateKey, "/v1/actions/checkout/create-session", checkoutRequest, 60)
	if err != nil {
		return nil, err
	}
	if session == nil || strings.TrimSpace(session.CheckoutURL) == "" || strings.TrimSpace(session.SessionID) == "" {
		return nil, &waffoPancakeActionError{
			StatusCode: http.StatusOK,
			Ambiguous:  true,
			Cause:      fmt.Errorf("checkout session response is missing critical fields"),
		}
	}
	return &WaffoPancakeCheckoutSession{
		SessionID:      session.SessionID,
		CheckoutURL:    session.CheckoutURL + "#token=" + token.Token,
		ExpiresAt:      session.ExpiresAt,
		Token:          token.Token,
		TokenExpiresAt: token.ExpiresAt,
	}, nil
}

func optionalString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}

// WaffoPancakeBuyerIdentityFromUserID renders the canonical buyer identity
// for checkout. Webhook handlers compare against the value rendered here to
// reject identity mismatches, so both call sites must use this function.
func WaffoPancakeBuyerIdentityFromUserID(userID int) string {
	return fmt.Sprintf("new-api-user-%d", userID)
}

// VerifyWaffoPancakeWebhookForEnvironment verifies a webhook with the public
// key for the environment encoded in the route, then requires the signed
// payload mode to match. Verification deliberately does not depend on mutable
// gateway settings so already-created orders remain settleable during config
// changes or compliance maintenance.
func VerifyWaffoPancakeWebhookForEnvironment(payload string, signatureHeader string, environment string) (*WaffoPancakeWebhookEvent, error) {
	environment = strings.TrimSpace(environment)
	if !setting.IsValidWaffoPancakeEnvironment(environment) {
		return nil, fmt.Errorf("invalid Waffo Pancake webhook environment: %q", environment)
	}
	evt, err := pancake.VerifyWebhookTyped[pancake.WebhookEventData](payload, signatureHeader, &pancake.VerifyWebhookOptions{
		Environment: pancake.Environment(environment),
	})
	if err != nil {
		return nil, err
	}
	if string(evt.Mode) != environment {
		return nil, fmt.Errorf("Waffo Pancake webhook mode mismatch: expected=%q actual=%q", environment, evt.Mode)
	}
	identity := ""
	if evt.Data.MerchantProvidedBuyerIdentity != nil {
		identity = *evt.Data.MerchantProvidedBuyerIdentity
	}
	externalID := ""
	if evt.Data.OrderMerchantExternalID != nil {
		externalID = *evt.Data.OrderMerchantExternalID
	}
	return &WaffoPancakeWebhookEvent{
		ID:        evt.ID,
		Timestamp: evt.Timestamp,
		EventType: evt.EventType,
		EventID:   evt.EventID,
		StoreID:   evt.StoreID,
		Mode:      string(evt.Mode),
		Data: WaffoPancakeWebhookData{
			OrderID:                       evt.Data.OrderID,
			OrderMerchantExternalID:       externalID,
			BuyerEmail:                    evt.Data.BuyerEmail,
			Currency:                      evt.Data.Currency,
			Amount:                        evt.Data.Amount,
			TaxAmount:                     evt.Data.TaxAmount,
			ProductName:                   evt.Data.ProductName,
			MerchantProvidedBuyerIdentity: identity,
		},
	}, nil
}

// ResolveWaffoPancakeTradeNo maps a verified webhook event to a local TopUp
// trade_no via OrderMerchantExternalID, and rejects buyer-identity mismatches.
func ResolveWaffoPancakeTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if tradeNo == "" {
		return "", fmt.Errorf("missing webhook orderMerchantExternalId")
	}
	topUp, err := model.FindTopUpByTradeNo(tradeNo)
	if err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) {
			return "", fmt.Errorf("waffo pancake order not found for tradeNo=%s", tradeNo)
		}
		return "", fmt.Errorf("%w for tradeNo=%s: %v", ErrWaffoPancakeOrderLookup, tradeNo, err)
	}
	if topUp.PaymentProvider != model.PaymentProviderWaffoPancake {
		return "", fmt.Errorf("waffo pancake order not found for tradeNo=%s", tradeNo)
	}
	if err := validateWaffoPancakeSettlement(event, topUp.UserId, topUp.ProviderStoreId, topUp.ProviderEnvironment, topUp.ProviderCurrency, topUp.Money); err != nil {
		return "", fmt.Errorf("waffo pancake settlement mismatch for tradeNo=%s: %w", tradeNo, err)
	}
	return tradeNo, nil
}

// ResolveWaffoPancakeSubscriptionTradeNo is the SubscriptionOrder counterpart
// of ResolveWaffoPancakeTradeNo.
func ResolveWaffoPancakeSubscriptionTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if tradeNo == "" {
		return "", fmt.Errorf("missing webhook orderMerchantExternalId")
	}
	order, err := model.FindSubscriptionOrderByTradeNo(tradeNo)
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionOrderNotFound) {
			return "", fmt.Errorf("waffo pancake subscription order not found for tradeNo=%s", tradeNo)
		}
		return "", fmt.Errorf("%w for subscription tradeNo=%s: %v", ErrWaffoPancakeOrderLookup, tradeNo, err)
	}
	if order.PaymentProvider != model.PaymentProviderWaffoPancake {
		return "", fmt.Errorf("waffo pancake subscription order not found for tradeNo=%s", tradeNo)
	}
	if err := validateWaffoPancakeSettlement(event, order.UserId, order.ProviderStoreId, order.ProviderEnvironment, order.ProviderCurrency, order.Money); err != nil {
		return "", fmt.Errorf("waffo pancake subscription settlement mismatch for tradeNo=%s: %w", tradeNo, err)
	}
	return tradeNo, nil
}

func validateWaffoPancakeSettlement(event *WaffoPancakeWebhookEvent, userID int, storeID, environment, currency string, money float64) error {
	if event == nil {
		return fmt.Errorf("missing webhook event")
	}
	expectedIdentity := WaffoPancakeBuyerIdentityFromUserID(userID)
	if actualIdentity := strings.TrimSpace(event.Data.MerchantProvidedBuyerIdentity); actualIdentity != expectedIdentity {
		return fmt.Errorf("buyer identity mismatch: expected=%q actual=%q", expectedIdentity, actualIdentity)
	}
	expectedStoreID := strings.TrimSpace(storeID)
	if expectedStoreID == "" {
		expectedStoreID = strings.TrimSpace(setting.WaffoPancakeStoreID)
		if expectedStoreID == "" {
			return fmt.Errorf("legacy order is missing a trusted store binding")
		}
	}
	if actualStoreID := strings.TrimSpace(event.StoreID); actualStoreID != expectedStoreID {
		return fmt.Errorf("store mismatch: expected=%q actual=%q", expectedStoreID, actualStoreID)
	}
	expectedEnvironment := strings.TrimSpace(environment)
	if expectedEnvironment == "" {
		expectedEnvironment = strings.TrimSpace(setting.WaffoPancakeEnvironment)
	}
	if !setting.IsValidWaffoPancakeEnvironment(expectedEnvironment) {
		return fmt.Errorf("legacy order is missing a trusted environment binding")
	}
	if actualMode := strings.TrimSpace(event.Mode); actualMode != expectedEnvironment {
		return fmt.Errorf("environment mismatch: expected=%q actual=%q", expectedEnvironment, actualMode)
	}
	expectedCurrency := strings.ToUpper(strings.TrimSpace(currency))
	if expectedCurrency == "" {
		expectedCurrency = "USD"
		if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeCNY {
			expectedCurrency = "CNY"
		}
	}
	actualCurrency := strings.ToUpper(strings.TrimSpace(event.Data.Currency))
	if actualCurrency != expectedCurrency {
		return fmt.Errorf("currency mismatch: expected=%q actual=%q", expectedCurrency, actualCurrency)
	}
	actualAmount, err := decimal.NewFromString(strings.TrimSpace(event.Data.Amount))
	if err != nil {
		return fmt.Errorf("invalid amount: %q", event.Data.Amount)
	}
	expectedAmount := decimal.NewFromFloat(money).Round(2)
	if !actualAmount.Equal(expectedAmount) {
		return fmt.Errorf("amount mismatch: expected=%s actual=%s", expectedAmount.StringFixed(2), actualAmount.String())
	}
	return nil
}

// Deterministic default names for "+ Create": stable bodies mean stable
// X-Idempotency-Key, which lets Pancake dedupe retries server-side.
const (
	defaultWaffoPancakeStoreName   = "new-api-store"
	defaultWaffoPancakeProductName = "new-api-charge-product"
)

// CreateWaffoPancakePrimaryStore creates a Pancake Store using in-flight
// (not-yet-persisted) credentials and returns the new store ID.
func CreateWaffoPancakePrimaryStore(ctx context.Context, merchantID, privateKey string) (string, error) {
	client, err := newWaffoPancakeClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	storeRes, err := client.Stores.Create(ctx, pancake.CreateStoreParams{
		Name: defaultWaffoPancakeStoreName,
	})
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake store: %w", err)
	}
	return storeRes.Store.ID, nil
}

// CreateWaffoPancakeProductForPlan mints a Pancake product and publishes it
// when the selected environment is production.
// OnetimeProduct priced at `amount` in USD and CNY, used as a subscription plan's
// SubscriptionPlan.WaffoPancakeProductId.
//
// OnetimeProduct (not SubscriptionProduct) because new-api has no renewal-
// event handling; Pancake auto-renewing without new-api extending user
// access would be a UX divergence. Revisit if renewal handling is added.
func CreateWaffoPancakeProductForPlan(ctx context.Context, merchantID, privateKey, storeID, name, amount, returnURL, environment string) (string, error) {
	if !setting.IsValidWaffoPancakeEnvironment(environment) {
		return "", fmt.Errorf("invalid Waffo Pancake environment: %q", environment)
	}
	storeID = strings.TrimSpace(storeID)
	if storeID == "" {
		return "", fmt.Errorf("store id is required to create a product")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("plan name is required")
	}
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return "", fmt.Errorf("plan price is required")
	}
	client, err := newWaffoPancakeClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	prices, err := waffoPancakePrices(amount)
	if err != nil {
		return "", err
	}
	prodRes, err := postWaffoPancakeAction[struct {
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
	}](ctx, merchantID, privateKey, "/v1/actions/onetime-product/create-product", struct {
		StoreID    string               `json:"storeId"`
		Name       string               `json:"name"`
		Prices     waffoPancakePriceMap `json:"prices"`
		SuccessURL *string              `json:"successUrl,omitempty"`
	}{
		StoreID:    storeID,
		Name:       name,
		Prices:     prices,
		SuccessURL: optionalString(strings.TrimSpace(returnURL)),
	}, 0)
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake plan product: %w", err)
	}
	if prodRes == nil || strings.TrimSpace(prodRes.Product.ID) == "" {
		return "", fmt.Errorf("create Waffo Pancake plan product: empty product id")
	}
	productID := prodRes.Product.ID
	if environment == setting.WaffoPancakeEnvironmentProd {
		if err := ensureWaffoPancakeProductProductionVersion(ctx, client, productID); err != nil {
			return "", fmt.Errorf("publish Waffo Pancake plan product: %w", err)
		}
	}
	return productID, nil
}

// CreateWaffoPancakePrimaryProduct mints the wallet-top-up product and
// publishes it when the selected environment is production.
// OnetimeProduct under storeID. Per-checkout price overrides via PriceSnapshot
// are what make the "1.00" seed price irrelevant at runtime.
func CreateWaffoPancakePrimaryProduct(ctx context.Context, merchantID, privateKey, storeID, returnURL, environment string) (string, error) {
	if !setting.IsValidWaffoPancakeEnvironment(environment) {
		return "", fmt.Errorf("invalid Waffo Pancake environment: %q", environment)
	}
	storeID = strings.TrimSpace(storeID)
	if storeID == "" {
		return "", fmt.Errorf("store id is required to create a product")
	}
	client, err := newWaffoPancakeClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	prices, err := waffoPancakePrices("1.00")
	if err != nil {
		return "", err
	}
	prodRes, err := postWaffoPancakeAction[struct {
		Product struct {
			ID string `json:"id"`
		} `json:"product"`
	}](ctx, merchantID, privateKey, "/v1/actions/onetime-product/create-product", struct {
		StoreID    string               `json:"storeId"`
		Name       string               `json:"name"`
		Prices     waffoPancakePriceMap `json:"prices"`
		SuccessURL *string              `json:"successUrl,omitempty"`
	}{
		StoreID:    storeID,
		Name:       defaultWaffoPancakeProductName,
		Prices:     prices, // overridden at checkout via PriceSnapshot
		SuccessURL: optionalString(strings.TrimSpace(returnURL)),
	}, 0)
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake product: %w", err)
	}
	if prodRes == nil || strings.TrimSpace(prodRes.Product.ID) == "" {
		return "", fmt.Errorf("create Waffo Pancake product: empty product id")
	}
	productID := prodRes.Product.ID
	if environment == setting.WaffoPancakeEnvironmentProd {
		if err := ensureWaffoPancakeProductProductionVersion(ctx, client, productID); err != nil {
			return "", fmt.Errorf("publish Waffo Pancake product: %w", err)
		}
	}
	return productID, nil
}

func ensureWaffoPancakeProductProductionVersion(ctx context.Context, client *pancake.Client, productID string) error {
	hasProductionVersion, err := waffoPancakeProductHasProductionVersion(ctx, client, productID)
	if err != nil {
		return err
	}
	if hasProductionVersion {
		return nil
	}

	publishResult, err := client.OnetimeProducts.Publish(ctx, pancake.PublishOnetimeProductParams{ID: productID})
	if err != nil {
		return err
	}
	if publishResult == nil || strings.TrimSpace(publishResult.Product.ID) != productID {
		return fmt.Errorf("Waffo Pancake publish returned an invalid product")
	}
	for _, warning := range publishResult.Warnings {
		common.SysLog(fmt.Sprintf("Waffo Pancake publish warning product_id=%q warning=%q", productID, warning.Message))
	}

	hasProductionVersion, err = waffoPancakeProductHasProductionVersion(ctx, client, productID)
	if err != nil {
		return err
	}
	if !hasProductionVersion {
		return fmt.Errorf("Waffo Pancake product %q has no production version after publish", productID)
	}
	return nil
}

func waffoPancakeProductHasProductionVersion(ctx context.Context, client *pancake.Client, productID string) (bool, error) {
	type queryShape struct {
		OnetimeProductVersions []struct {
			IsProdVersion bool `json:"isProdVersion"`
		} `json:"onetimeProductVersions"`
	}

	response, err := pancake.GraphQLQuery[queryShape](ctx, client, pancake.GraphQLParams{
		Query: `query ($productId: String!) {
			onetimeProductVersions(productId: $productId) {
				isProdVersion
			}
		}`,
		Variables: map[string]any{"productId": productID},
	})
	if err != nil {
		return false, fmt.Errorf("query Waffo Pancake product versions: %w", err)
	}
	if len(response.Errors) > 0 {
		return false, fmt.Errorf("query Waffo Pancake product versions: %s", response.Errors[0].Message)
	}
	for _, warning := range response.Warnings {
		common.SysLog(fmt.Sprintf("Waffo Pancake product-version query warning product_id=%q warning=%q", productID, warning.Message))
	}
	for _, version := range response.Data.OnetimeProductVersions {
		if version.IsProdVersion {
			return true, nil
		}
	}
	return false, nil
}

func waffoPancakePrices(usdAmount string) (waffoPancakePriceMap, error) {
	usdPrice, err := decimal.NewFromString(usdAmount)
	if err != nil || !usdPrice.IsPositive() {
		return nil, fmt.Errorf("invalid Waffo Pancake USD price: %q", usdAmount)
	}
	if operation_setting.USDExchangeRate <= 0 {
		return nil, fmt.Errorf("invalid USD to CNY exchange rate: %v", operation_setting.USDExchangeRate)
	}

	usd := waffoPancakePriceInfo{
		Amount:      usdPrice.StringFixed(2),
		TaxIncluded: true,
		TaxCategory: "saas",
	}
	cny := usd
	cny.Amount = usdPrice.
		Mul(decimal.NewFromFloat(operation_setting.USDExchangeRate)).
		StringFixed(2)
	return waffoPancakePriceMap{"USD": usd, "CNY": cny}, nil
}

// WaffoPancakePairResult is the response of CreateWaffoPancakePrimaryPair.
// When OrphanStore is true the store was created but the product wasn't,
// so the caller can surface a partial-failure message with StoreID.
type WaffoPancakePairResult struct {
	StoreID     string
	StoreName   string
	ProductID   string
	ProductName string
	OrphanStore bool
}

// CreateWaffoPancakePrimaryPair mints a Store + OnetimeProduct in one
// round-trip — the canonical "+ Create" entry point. Nothing is persisted
// to settings; the operator's final Save commits the chosen IDs.
func CreateWaffoPancakePrimaryPair(ctx context.Context, merchantID, privateKey, returnURL, environment string) (*WaffoPancakePairResult, error) {
	if !setting.IsValidWaffoPancakeEnvironment(environment) {
		return nil, fmt.Errorf("invalid Waffo Pancake environment: %q", environment)
	}
	storeID, err := CreateWaffoPancakePrimaryStore(ctx, merchantID, privateKey)
	if err != nil {
		return nil, err
	}
	productID, err := CreateWaffoPancakePrimaryProduct(ctx, merchantID, privateKey, storeID, returnURL, environment)
	if err != nil {
		return &WaffoPancakePairResult{
			StoreID:     storeID,
			StoreName:   defaultWaffoPancakeStoreName,
			OrphanStore: true,
		}, fmt.Errorf("store created at %s but product creation failed: %w", storeID, err)
	}
	return &WaffoPancakePairResult{
		StoreID:     storeID,
		StoreName:   defaultWaffoPancakeStoreName,
		ProductID:   productID,
		ProductName: defaultWaffoPancakeProductName,
	}, nil
}

// SaveWaffoPancakeConfig persists the operator-controlled fields atomically
// at the end of the configuration flow via model.UpdateOptionsBulk (single
// DB transaction). A blank privateKey is treated as "keep current"
// (Stripe-style API-secret UX) and is omitted from the bulk payload.
func SaveWaffoPancakeConfig(ctx context.Context, merchantID, privateKey, returnURL, storeID, productID, environment string) error {
	merchantID = strings.TrimSpace(merchantID)
	storeID = strings.TrimSpace(storeID)
	productID = strings.TrimSpace(productID)
	if merchantID == "" || storeID == "" || productID == "" {
		return fmt.Errorf("merchant id, store id, and product id are required to save")
	}
	effectivePrivateKey := strings.TrimSpace(privateKey)
	if effectivePrivateKey == "" {
		effectivePrivateKey = strings.TrimSpace(setting.WaffoPancakePrivateKey)
	}
	if effectivePrivateKey == "" {
		return fmt.Errorf("private key is required to save")
	}

	catalog, err := ListWaffoPancakeCatalog(ctx, merchantID, effectivePrivateKey)
	if err != nil {
		return fmt.Errorf("validate Waffo Pancake credentials: %w", err)
	}
	requestedEnvironment := strings.TrimSpace(environment)
	if requestedEnvironment != "" && requestedEnvironment != catalog.Environment {
		return fmt.Errorf("Waffo Pancake key environment mismatch: requested=%q actual=%q", requestedEnvironment, catalog.Environment)
	}
	if err := catalog.ValidateBinding(storeID, productID); err != nil {
		return err
	}
	values := map[string]string{
		"WaffoPancakeMerchantID":  merchantID,
		"WaffoPancakeReturnURL":   strings.TrimSpace(returnURL),
		"WaffoPancakeStoreID":     storeID,
		"WaffoPancakeProductID":   productID,
		"WaffoPancakeEnvironment": catalog.Environment,
	}
	if pk := strings.TrimSpace(privateKey); pk != "" {
		values["WaffoPancakePrivateKey"] = pk
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		return fmt.Errorf("persist Waffo Pancake config: %w", err)
	}
	return nil
}

type WaffoPancakeCatalogProduct struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// WaffoPancakeCatalogStore nests its OnetimeProducts so the UI can render a
// dependent store→product select without a second round-trip.
type WaffoPancakeCatalogStore struct {
	ID              string                       `json:"id"`
	Name            string                       `json:"name"`
	Status          string                       `json:"status"`
	ProdEnabled     bool                         `json:"prodEnabled"`
	OnetimeProducts []WaffoPancakeCatalogProduct `json:"onetimeProducts"`
}

type WaffoPancakeCatalog struct {
	Environment string                     `json:"environment"`
	Stores      []WaffoPancakeCatalogStore `json:"stores"`
}

func (catalog *WaffoPancakeCatalog) ValidateBinding(storeID, productID string) error {
	if err := catalog.ValidateStore(storeID); err != nil {
		return err
	}
	storeID = strings.TrimSpace(storeID)
	productID = strings.TrimSpace(productID)
	for _, store := range catalog.Stores {
		if strings.TrimSpace(store.ID) != storeID {
			continue
		}
		for _, product := range store.OnetimeProducts {
			if strings.TrimSpace(product.ID) == productID {
				return nil
			}
		}
		return fmt.Errorf("Waffo Pancake product %q is not active in store %q", productID, storeID)
	}
	return fmt.Errorf("Waffo Pancake store %q was not found", storeID)
}

func (catalog *WaffoPancakeCatalog) ValidateStore(storeID string) error {
	if catalog == nil || !setting.IsValidWaffoPancakeEnvironment(catalog.Environment) {
		return fmt.Errorf("Waffo Pancake key environment could not be determined")
	}
	storeID = strings.TrimSpace(storeID)
	for _, store := range catalog.Stores {
		if strings.TrimSpace(store.ID) != storeID {
			continue
		}
		if status := strings.TrimSpace(store.Status); status != "" && !strings.EqualFold(status, "active") {
			return fmt.Errorf("Waffo Pancake store %q is not active", storeID)
		}
		if catalog.Environment == setting.WaffoPancakeEnvironmentProd && !store.ProdEnabled {
			return fmt.Errorf("Waffo Pancake store %q is not approved for production", storeID)
		}
		return nil
	}
	return fmt.Errorf("Waffo Pancake store %q was not found", storeID)
}

func (catalog *WaffoPancakeCatalog) ResolveActiveProductStore(productID string) (string, error) {
	if catalog == nil || !setting.IsValidWaffoPancakeEnvironment(catalog.Environment) {
		return "", fmt.Errorf("Waffo Pancake key environment could not be determined")
	}
	productID = strings.TrimSpace(productID)
	for _, store := range catalog.Stores {
		if status := strings.TrimSpace(store.Status); status != "" && !strings.EqualFold(status, "active") {
			continue
		}
		if catalog.Environment == setting.WaffoPancakeEnvironmentProd && !store.ProdEnabled {
			continue
		}
		for _, product := range store.OnetimeProducts {
			if strings.TrimSpace(product.ID) == productID {
				return strings.TrimSpace(store.ID), nil
			}
		}
	}
	return "", fmt.Errorf("Waffo Pancake product %q is not active in the key environment", productID)
}

// ListWaffoPancakeCatalog queries Pancake's GraphQL `stores` for the
// merchant's stores + onetime products. A successful call also proves
// the supplied credentials authenticate (doubles as a credential probe).
func ListWaffoPancakeCatalog(ctx context.Context, merchantID, privateKey string) (*WaffoPancakeCatalog, error) {
	client, err := newWaffoPancakeClientFromCreds(merchantID, privateKey)
	if err != nil {
		return nil, err
	}

	type queryShape struct {
		APIKeys []struct {
			PrivateKey  string `json:"privateKey"`
			Environment string `json:"environment"`
		} `json:"apiKeys"`
		Stores []WaffoPancakeCatalogStore `json:"stores"`
	}
	// `limit: 100` because the API returns a single store when limit is
	// omitted, even for multi-store merchants. Bump to paginated fetches
	// (via `offset`) if real catalogs ever cross the cap.
	resp, err := pancake.GraphQLQuery[queryShape](ctx, client, pancake.GraphQLParams{
		Query: `query {
			apiKeys {
				privateKey
				environment
			}
			stores(limit: 100) {
				id
				name
				status
				prodEnabled
				onetimeProducts {
					id
					name
					status
				}
			}
		}`,
	})
	if err != nil {
		return nil, fmt.Errorf("query Waffo Pancake catalog: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("waffo pancake catalog query returned %d errors: %s",
			len(resp.Errors), resp.Errors[0].Message)
	}
	privateKeyFingerprint := normalizeWaffoPancakePrivateKey(privateKey)
	detectedEnvironment := ""
	for _, apiKey := range resp.Data.APIKeys {
		if normalizeWaffoPancakePrivateKey(apiKey.PrivateKey) != privateKeyFingerprint {
			continue
		}
		candidate := strings.ToLower(strings.TrimSpace(apiKey.Environment))
		if !setting.IsValidWaffoPancakeEnvironment(candidate) {
			return nil, fmt.Errorf("Waffo Pancake API key has invalid environment: %q", candidate)
		}
		if detectedEnvironment != "" && detectedEnvironment != candidate {
			return nil, fmt.Errorf("Waffo Pancake API key matched conflicting environments")
		}
		detectedEnvironment = candidate
	}
	if detectedEnvironment == "" {
		return nil, fmt.Errorf("Waffo Pancake API key environment could not be determined")
	}

	// Drop non-active products. Operators should only see items they can
	// actually bind without later hitting "product unavailable" at checkout.
	stores := resp.Data.Stores
	for i := range stores {
		active := stores[i].OnetimeProducts[:0]
		for _, p := range stores[i].OnetimeProducts {
			if strings.EqualFold(strings.TrimSpace(p.Status), "active") {
				active = append(active, p)
			}
		}
		stores[i].OnetimeProducts = active
	}
	return &WaffoPancakeCatalog{
		Environment: detectedEnvironment,
		Stores:      stores,
	}, nil
}

func normalizeWaffoPancakePrivateKey(privateKey string) string {
	privateKey = strings.ReplaceAll(privateKey, `\n`, "\n")
	for _, marker := range []string{
		"-----BEGIN PRIVATE KEY-----",
		"-----END PRIVATE KEY-----",
		"-----BEGIN RSA PRIVATE KEY-----",
		"-----END RSA PRIVATE KEY-----",
	} {
		privateKey = strings.ReplaceAll(privateKey, marker, "")
	}
	return strings.Join(strings.Fields(privateKey), "")
}
