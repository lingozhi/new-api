package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

type WaffoPancakePayRequest struct {
	Amount int64 `json:"amount"`
}

func RequestWaffoPancakeAmount(c *gin.Context) {
	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if err := validateWaffoPancakeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": fmt.Sprintf("%.2f", payMoney)})
}

func validateWaffoPancakeTopUpAmount(amount int64) error {
	if amount < int64(setting.WaffoPancakeMinTopUp) {
		return fmt.Errorf("充值数量不能小于 %d", setting.WaffoPancakeMinTopUp)
	}
	if amount > maxOnlineTopUpAmount {
		return fmt.Errorf("充值数量不能大于 %d", maxOnlineTopUpAmount)
	}
	return nil
}

func getWaffoPancakePayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && ds > 0 {
		discount = ds
	}

	payMoney := dAmount.
		Mul(decimal.NewFromFloat(setting.WaffoPancakeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))
	if operation_setting.UseCNYTopUpAmounts() {
		payMoney = dAmount.
			Mul(decimal.NewFromFloat(topupGroupRatio)).
			Mul(decimal.NewFromFloat(discount))
		return payMoney.Round(2).InexactFloat64()
	}

	return getWaffoPancakeCheckoutAmount(payMoney.InexactFloat64())
}

func getWaffoPancakeCheckoutAmount(usdAmount float64) float64 {
	amount := decimal.NewFromFloat(usdAmount)
	if getWaffoPancakeCheckoutCurrency() != "CNY" {
		return amount.Round(2).InexactFloat64()
	}
	return amount.
		Mul(decimal.NewFromFloat(operation_setting.USDExchangeRate)).
		Round(2).
		InexactFloat64()
}

func getWaffoPancakeCheckoutCurrency() string {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeCNY {
		return "CNY"
	}
	return "USD"
}

func normalizeWaffoPancakeTopUpAmount(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}

	normalized := decimal.NewFromInt(amount).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		IntPart()
	if normalized < 1 {
		return 1
	}
	return normalized
}

func formatWaffoPancakeAmount(payMoney float64) string {
	return decimal.NewFromFloat(payMoney).StringFixed(2)
}

func getWaffoPancakeBuyerEmail(user *model.User) string {
	if user != nil && strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	return ""
}

// The admin config endpoints below accept typed-but-not-yet-saved creds in
// the body and fall back to persisted creds when the body is blank (see
// resolveWaffoPancakeAdminCreds). Only SaveWaffoPancake writes to OptionMap.

type saveWaffoPancakeRequest struct {
	MerchantID  string `json:"merchant_id"`
	PrivateKey  string `json:"private_key"`
	ReturnURL   string `json:"return_url"`
	StoreID     string `json:"store_id"`
	ProductID   string `json:"product_id"`
	Environment string `json:"environment"`
}

type createWaffoPancakePairRequest struct {
	MerchantID  string `json:"merchant_id"`
	PrivateKey  string `json:"private_key"`
	ReturnURL   string `json:"return_url"`
	Environment string `json:"environment"`
}

// SaveWaffoPancake atomically persists all operator-controlled fields.
// Catalog / pair endpoints are transient — only this one writes the OptionMap.
func SaveWaffoPancake(c *gin.Context) {
	var req saveWaffoPancakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if err := service.SaveWaffoPancakeConfig(
		c.Request.Context(),
		req.MerchantID,
		req.PrivateKey,
		req.ReturnURL,
		req.StoreID,
		req.ProductID,
		req.Environment,
	); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 保存配置失败 store_id=%q product_id=%q error=%q",
			req.StoreID, req.ProductID, err.Error(),
		))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "保存配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"product_id":  setting.WaffoPancakeProductID,
			"store_id":    setting.WaffoPancakeStoreID,
			"environment": setting.WaffoPancakeEnvironment,
		},
	})
}

// resolveWaffoPancakeAdminCreds prefers body creds (typed-but-not-yet-saved
// values, for verification) and falls back per field to persisted creds when
// blank (so returning admins don't have to re-paste the private key,
// which is stripped from GET /api/option/).
func resolveWaffoPancakeAdminCreds(bodyMerchantID, bodyPrivateKey string) (string, string) {
	m := strings.TrimSpace(bodyMerchantID)
	k := strings.TrimSpace(bodyPrivateKey)
	if m == "" {
		m = strings.TrimSpace(setting.WaffoPancakeMerchantID)
	}
	if k == "" {
		k = strings.TrimSpace(setting.WaffoPancakePrivateKey)
	}
	return m, k
}

func requireWaffoPancakeEnvironmentMatch(expected, actual string) error {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if !setting.IsValidWaffoPancakeEnvironment(actual) {
		return fmt.Errorf("invalid detected Waffo Pancake environment: %q", actual)
	}
	if expected != "" && expected != actual {
		return fmt.Errorf("Waffo Pancake environment mismatch: expected=%q actual=%q", expected, actual)
	}
	return nil
}

// CreateWaffoPancakePair mints a Store + OnetimeProduct pair in one round-
// trip. Surfaces an orphan-store flag when the product half fails so the
// frontend can preselect / retry without losing context.
func CreateWaffoPancakePair(c *gin.Context) {
	var req createWaffoPancakePairRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
			return
		}
	}
	merchantID, privateKey := resolveWaffoPancakeAdminCreds(req.MerchantID, req.PrivateKey)
	if merchantID == "" || privateKey == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 凭证未配置"})
		return
	}
	catalog, err := service.ListWaffoPancakeCatalog(c.Request.Context(), merchantID, privateKey)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建前凭证校验失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 凭证校验失败"})
		return
	}
	if err := requireWaffoPancakeEnvironmentMatch(req.Environment, catalog.Environment); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 密钥环境不匹配"})
		return
	}
	result, err := service.CreateWaffoPancakePrimaryPair(
		c.Request.Context(), merchantID, privateKey, req.ReturnURL, catalog.Environment,
	)
	if err != nil {
		orphan := result != nil && result.OrphanStore
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 创建店铺与产品失败 orphan_store=%t store_id=%q error=%q",
			orphan, func() string {
				if result == nil {
					return ""
				}
				return result.StoreID
			}(), err.Error(),
		))
		data := gin.H{"error": err.Error()}
		if orphan {
			data["store_id"] = result.StoreID
			data["store_name"] = result.StoreName
			data["orphan_store"] = true
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": data})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"store_id":     result.StoreID,
			"store_name":   result.StoreName,
			"product_id":   result.ProductID,
			"product_name": result.ProductName,
			"environment":  catalog.Environment,
		},
	})
}

// ListWaffoPancakeCatalog returns the merchant's Stores + OnetimeProducts.
// Doubles as a credential probe (a successful 200 proves the resolved creds
// authenticate). See resolveWaffoPancakeAdminCreds for credential resolution.
func ListWaffoPancakeCatalog(c *gin.Context) {
	var req createWaffoPancakePairRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	// Missing body creds mean "use persisted creds". Credentials must never be
	// accepted from the query string because URLs can be retained in access logs.
	merchantID, privateKey := resolveWaffoPancakeAdminCreds(req.MerchantID, req.PrivateKey)
	if merchantID == "" || privateKey == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 凭证未配置"})
		return
	}
	catalog, err := service.ListWaffoPancakeCatalog(c.Request.Context(), merchantID, privateKey)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 拉取店铺与产品目录失败 error=%q", err.Error(),
		))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉取目录失败"})
		return
	}
	if err := requireWaffoPancakeEnvironmentMatch(req.Environment, catalog.Environment); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 密钥环境不匹配"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": catalog})
}

func RejectLegacyWaffoPancakeCatalog(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"message": "error",
		"data":    "Waffo Pancake 目录接口已改用 POST",
	})
}

type createWaffoPancakeSubscriptionProductRequest struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// CreateWaffoPancakeSubscriptionProduct mints an OnetimeProduct (not
// SubscriptionProduct — see service.CreateWaffoPancakeProductForPlan)
// sized to a plan's `name` + `amount`, using persisted Pancake credentials
// + StoreID. Reads from the form, not the plan row, so newly-typed unsaved
// plans can mint a product too.
func CreateWaffoPancakeSubscriptionProduct(c *gin.Context) {
	var req createWaffoPancakeSubscriptionProductRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
			return
		}
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "套餐名称不能为空"})
		return
	}
	if strings.TrimSpace(req.Amount) == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "套餐价格不能为空"})
		return
	}
	merchantID, privateKey := resolveWaffoPancakeAdminCreds("", "")
	storeID := strings.TrimSpace(setting.WaffoPancakeStoreID)
	if merchantID == "" || privateKey == "" || storeID == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 未完成配置，请先在支付设置中完成网关绑定"})
		return
	}
	catalog, err := service.ListWaffoPancakeCatalog(c.Request.Context(), merchantID, privateKey)
	if err != nil || requireWaffoPancakeEnvironmentMatch(setting.WaffoPancakeEnvironment, catalog.Environment) != nil {
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建套餐产品前凭证校验失败 error=%q", err.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 配置环境无效"})
		return
	}
	if err := catalog.ValidateStore(storeID); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建套餐产品前店铺校验失败 store_id=%q error=%q", storeID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 店铺不可用"})
		return
	}
	productID, err := service.CreateWaffoPancakeProductForPlan(
		c.Request.Context(),
		merchantID,
		privateKey,
		storeID,
		req.Name,
		req.Amount,
		setting.WaffoPancakeReturnURL,
		catalog.Environment,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 创建套餐产品失败 store_id=%q name=%q amount=%q error=%q",
			storeID, req.Name, req.Amount, err.Error(),
		))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建套餐产品失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"product_id":   productID,
			"product_name": req.Name,
			"store_id":     storeID,
		},
	})
}

// ListWaffoPancakeSubscriptionProductOptions returns the OnetimeProducts
// in the saved Pancake store, for the subscription-plan dropdown. The name
// reflects new-api's plan concept; under the hood it's still OnetimeProducts.
func ListWaffoPancakeSubscriptionProductOptions(c *gin.Context) {
	merchantID, privateKey := resolveWaffoPancakeAdminCreds("", "")
	storeID := strings.TrimSpace(setting.WaffoPancakeStoreID)
	if merchantID == "" || privateKey == "" || storeID == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 未完成配置，请先在支付设置中完成网关绑定"})
		return
	}
	catalog, err := service.ListWaffoPancakeCatalog(c.Request.Context(), merchantID, privateKey)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 拉取订阅产品列表失败 store_id=%q error=%q", storeID, err.Error(),
		))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉取产品列表失败"})
		return
	}
	if err := requireWaffoPancakeEnvironmentMatch(setting.WaffoPancakeEnvironment, catalog.Environment); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 配置环境无效"})
		return
	}
	if err := catalog.ValidateStore(storeID); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 店铺不可用"})
		return
	}
	products := []service.WaffoPancakeCatalogProduct{}
	for _, store := range catalog.Stores {
		if store.ID == storeID {
			products = store.OnetimeProducts
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"store_id":    storeID,
			"environment": catalog.Environment,
			"products":    products,
		},
	})
}

func getWaffoPancakeBuyerIdentity(user *model.User) string {
	if user == nil {
		return ""
	}
	return service.WaffoPancakeBuyerIdentityFromUserID(user.Id)
}

func RequestWaffoPancakePay(c *gin.Context) {
	if !isWaffoPancakeTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 配置不完整"})
		return
	}

	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if err := validateWaffoPancakeTopUpAmount(req.Amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	quotaAmount, err := calculateTopUpQuotaAmount(req.Amount)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值配置无效"})
		return
	}

	storeID := strings.TrimSpace(setting.WaffoPancakeStoreID)
	productID := strings.TrimSpace(setting.WaffoPancakeProductID)
	catalog, err := service.ListWaffoPancakeCatalog(
		c.Request.Context(),
		setting.WaffoPancakeMerchantID,
		setting.WaffoPancakePrivateKey,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值前凭证校验失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付配置校验失败"})
		return
	}
	if err := requireWaffoPancakeEnvironmentMatch(setting.WaffoPancakeEnvironment, catalog.Environment); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值前环境校验失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付配置环境无效"})
		return
	}
	if err := catalog.ValidateBinding(storeID, productID); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值前商品校验失败 user_id=%d store_id=%q product_id=%q error=%q", id, storeID, productID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付商品不可用"})
		return
	}

	tradeNo := fmt.Sprintf("WAFFO_PANCAKE-%d-%d-%s", id, time.Now().UnixMilli(), randstr.String(6))
	currency := getWaffoPancakeCheckoutCurrency()
	topUp := &model.TopUp{
		UserId:              id,
		Amount:              normalizeWaffoPancakeTopUpAmount(req.Amount),
		QuotaAmount:         int64(quotaAmount),
		Money:               payMoney,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodWaffoPancake,
		PaymentProvider:     model.PaymentProviderWaffoPancake,
		ProviderStoreId:     storeID,
		ProviderEnvironment: catalog.Environment,
		ProviderCurrency:    currency,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     productID,
		Currency:      currency,
		BuyerIdentity: getWaffoPancakeBuyerIdentity(user),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      formatWaffoPancakeAmount(payMoney),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建结账会话失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		if service.IsWaffoPancakeActionOutcomeAmbiguous(err) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建结账会话结果不确定，订单保持待处理 user_id=%d trade_no=%s", id, tradeNo))
		} else if updateErr := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderWaffoPancake, common.TopUpStatusFailed); updateErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值订单标记失败异常 user_id=%d trade_no=%s checkout_error=%q update_error=%q", id, tradeNo, err.Error(), updateErr.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值订单创建成功 user_id=%d trade_no=%s session_id=%s amount=%d money=%.2f", id, tradeNo, session.SessionID, req.Amount, payMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":     session.CheckoutURL,
			"session_id":       session.SessionID,
			"expires_at":       session.ExpiresAt,
			"order_id":         tradeNo,
			"token":            session.Token,
			"token_expires_at": session.TokenExpiresAt,
		},
	})
}

func WaffoPancakeWebhook(c *gin.Context) {
	requestPath := c.Request.URL.Path
	// :env splits test vs prod traffic at the routing layer — operator
	// registers each URL in the matching webhook slot in Pancake's dashboard.
	// Signature verification also enforces event.mode == expectedEnv.
	expectedEnv := strings.TrimSpace(c.Param("env"))
	if expectedEnv != "test" && expectedEnv != "prod" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake webhook 路径环境段无效 env=%q path=%q client_ip=%s",
			expectedEnv, requestPath, c.ClientIP(),
		))
		c.String(http.StatusNotFound, "unknown env")
		return
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 读取请求体失败 path=%q client_ip=%s error=%q", requestPath, c.ClientIP(), err.Error()))
		c.String(http.StatusBadRequest, "bad request")
		return
	}

	signature := c.GetHeader("X-Waffo-Signature")
	event, err := service.VerifyWaffoPancakeWebhookForEnvironment(string(bodyBytes), signature, expectedEnv)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 验签失败 path=%q client_ip=%s error=%q", requestPath, c.ClientIP(), err.Error()))
		c.String(http.StatusUnauthorized, "invalid signature")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 验签成功 event_id=%s order_id=%s", event.ID, event.Data.OrderID))
	if event.NormalizedEventType() != "order.completed" {
		c.String(http.StatusOK, "OK")
		return
	}

	// Dispatch by trade_no prefix. OrderMerchantExternalID = our trade_no;
	// OrderID is Pancake's internal ORD_* (logs only).
	rawTradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	isSubscription := strings.HasPrefix(rawTradeNo, "WAFFO_PANCAKE_SUB-")

	if isSubscription {
		tradeNo, err := service.ResolveWaffoPancakeSubscriptionTradeNo(event)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf(
				"Waffo Pancake webhook 订阅订单解析失败 event_id=%s order_id=%s error=%q",
				event.ID, event.Data.OrderID, err.Error(),
			))
			if errors.Is(err, service.ErrWaffoPancakeOrderLookup) {
				c.String(http.StatusInternalServerError, "retry")
				return
			}
			c.String(http.StatusOK, "OK")
			return
		}
		LockOrder(tradeNo)
		defer UnlockOrder(tradeNo)
		if err := model.CompleteSubscriptionOrder(tradeNo, string(bodyBytes), model.PaymentProviderWaffoPancake, ""); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅完成失败 trade_no=%s event_id=%s order_id=%s client_ip=%s error=%q", tradeNo, event.ID, event.Data.OrderID, c.ClientIP(), err.Error()))
			c.String(http.StatusInternalServerError, "retry")
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅完成 trade_no=%s event_id=%s order_id=%s client_ip=%s", tradeNo, event.ID, event.Data.OrderID, c.ClientIP()))
		c.String(http.StatusOK, "OK")
		return
	}

	tradeNo, err := service.ResolveWaffoPancakeTradeNo(event)
	if err != nil {
		// LogError (not LogWarn): covers order-not-found and buyer-identity
		// mismatch — both warrant human attention. 200 OK so Waffo doesn't
		// retry a permanently-unresolvable webhook.
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake webhook 订单解析失败 event_id=%s order_id=%s error=%q",
			event.ID, event.Data.OrderID, err.Error(),
		))
		if errors.Is(err, service.ErrWaffoPancakeOrderLookup) {
			c.String(http.StatusInternalServerError, "retry")
			return
		}
		c.String(http.StatusOK, "OK")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	if err := model.RechargeWaffoPancake(tradeNo); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值处理失败 trade_no=%s event_id=%s order_id=%s client_ip=%s error=%q", tradeNo, event.ID, event.Data.OrderID, c.ClientIP(), err.Error()))
		c.String(http.StatusInternalServerError, "retry")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值成功 trade_no=%s event_id=%s order_id=%s client_ip=%s", tradeNo, event.ID, event.Data.OrderID, c.ClientIP()))
	c.String(http.StatusOK, "OK")
}
