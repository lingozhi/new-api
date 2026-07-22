package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

type SubscriptionWaffoPancakePayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestWaffoPancakePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionWaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if strings.TrimSpace(plan.WaffoPancakeProductId) == "" {
		common.ApiErrorMsg(c, "该套餐未配置 WaffoPancakeProductId")
		return
	}
	// Plan targets its own Pancake product, so we only require credentials
	// here — not the gateway-level WaffoPancakeProductID.
	if strings.TrimSpace(setting.WaffoPancakeMerchantID) == "" ||
		strings.TrimSpace(setting.WaffoPancakePrivateKey) == "" ||
		!setting.IsValidWaffoPancakeEnvironment(setting.WaffoPancakeEnvironment) {
		common.ApiErrorMsg(c, "Waffo Pancake 未配置或密钥无效")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	catalog, err := service.ListWaffoPancakeCatalog(
		c.Request.Context(),
		setting.WaffoPancakeMerchantID,
		setting.WaffoPancakePrivateKey,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅前凭证校验失败 user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		common.ApiErrorMsg(c, "支付配置校验失败")
		return
	}
	if err := requireWaffoPancakeEnvironmentMatch(setting.WaffoPancakeEnvironment, catalog.Environment); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅前环境校验失败 user_id=%d plan_id=%d error=%q", userId, plan.Id, err.Error()))
		common.ApiErrorMsg(c, "支付配置环境无效")
		return
	}
	storeID, err := catalog.ResolveActiveProductStore(plan.WaffoPancakeProductId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅前商品校验失败 user_id=%d plan_id=%d product_id=%q error=%q", userId, plan.Id, plan.WaffoPancakeProductId, err.Error()))
		common.ApiErrorMsg(c, "套餐支付商品不可用")
		return
	}

	// WAFFO_PANCAKE_SUB- prefix (vs. wallet's WAFFO_PANCAKE-) drives webhook
	// dispatch in WaffoPancakeWebhook.
	tradeNo := fmt.Sprintf("WAFFO_PANCAKE_SUB-%d-%d-%s", userId, time.Now().UnixMilli(), randstr.String(6))
	checkoutAmount := getWaffoPancakeCheckoutAmount(plan.PriceAmount)
	currency := getWaffoPancakeCheckoutCurrency()

	order := &model.SubscriptionOrder{
		UserId:              userId,
		PlanId:              plan.Id,
		Money:               checkoutAmount,
		TradeNo:             tradeNo,
		PaymentMethod:       model.PaymentMethodWaffoPancake,
		PaymentProvider:     model.PaymentProviderWaffoPancake,
		ProviderStoreId:     storeID,
		ProviderEnvironment: catalog.Environment,
		ProviderCurrency:    currency,
		CreateTime:          time.Now().Unix(),
		Status:              common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     plan.WaffoPancakeProductId,
		Currency:      currency,
		BuyerIdentity: service.WaffoPancakeBuyerIdentityFromUserID(user.Id),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      decimal.NewFromFloat(checkoutAmount).StringFixed(2),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅结账会话创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		if service.IsWaffoPancakeActionOutcomeAmbiguous(err) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅结账会话结果不确定，订单保持待处理 user_id=%d plan_id=%d trade_no=%s", userId, plan.Id, tradeNo))
		} else if updateErr := model.UpdatePendingSubscriptionOrderStatus(tradeNo, model.PaymentProviderWaffoPancake, common.TopUpStatusFailed); updateErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单标记失败异常 user_id=%d plan_id=%d trade_no=%s checkout_error=%q update_error=%q", userId, plan.Id, tradeNo, err.Error(), updateErr.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s session_id=%s money=%.2f", userId, plan.Id, tradeNo, session.SessionID, checkoutAmount))

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
