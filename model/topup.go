package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id                  int     `json:"id"`
	UserId              int     `json:"user_id" gorm:"index"`
	Amount              int64   `json:"amount"`
	QuotaAmount         int64   `json:"quota_amount"`
	AmountCurrency      string  `json:"amount_currency" gorm:"type:varchar(8);default:''"`
	Money               float64 `json:"money"`
	TradeNo             string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod       string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider     string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	ProviderStoreId     string  `json:"provider_store_id" gorm:"type:varchar(128);default:''"`
	ProviderEnvironment string  `json:"provider_environment" gorm:"type:varchar(8);default:''"`
	ProviderCurrency    string  `json:"provider_currency" gorm:"type:varchar(8);default:''"`
	CreateTime          int64   `json:"create_time"`
	CompleteTime        int64   `json:"complete_time"`
	Status              string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

type TopUpSettlement struct {
	Amount           string
	Currency         string
	PaymentRequestID string
}

func validateTopUpSettlement(topUp *TopUp, settlement TopUpSettlement, requireIdentity bool) error {
	actualAmount, err := decimal.NewFromString(strings.TrimSpace(settlement.Amount))
	if err != nil {
		return errors.New("支付回调金额无效")
	}
	expectedAmount := decimal.NewFromFloat(topUp.Money).Round(2)
	if !actualAmount.Round(2).Equal(expectedAmount) {
		return fmt.Errorf("支付回调金额不匹配: expected=%s actual=%s", expectedAmount.StringFixed(2), actualAmount.String())
	}
	if !requireIdentity {
		return nil
	}
	if settlement.PaymentRequestID != topUp.TradeNo {
		return errors.New("支付请求标识不匹配")
	}
	expectedCurrency := strings.ToUpper(strings.TrimSpace(topUp.ProviderCurrency))
	actualCurrency := strings.ToUpper(strings.TrimSpace(settlement.Currency))
	if expectedCurrency == "" {
		if len(actualCurrency) != 3 {
			return fmt.Errorf("支付回调币种无效: actual=%q", actualCurrency)
		}
		for _, char := range actualCurrency {
			if char < 'A' || char > 'Z' {
				return fmt.Errorf("支付回调币种无效: actual=%q", actualCurrency)
			}
		}
		return nil
	}
	if actualCurrency != expectedCurrency {
		return fmt.Errorf("支付回调币种不匹配: expected=%q actual=%q", expectedCurrency, actualCurrency)
	}
	return nil
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	topUp, err := FindTopUpByTradeNo(tradeNo)
	if err != nil {
		return nil
	}
	return topUp
}

func FindTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	if tradeNo == "" {
		return nil, ErrTopUpNotFound
	}
	var topUp TopUp
	if err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	return &topUp, nil
}

func getTopUpQuotaToCredit(topUp *TopUp) (int, error) {
	if topUp == nil {
		return 0, errors.New("充值订单不存在")
	}
	if topUp.QuotaAmount != 0 {
		if topUp.QuotaAmount < 0 || topUp.QuotaAmount > int64(common.MaxQuota) {
			return 0, errors.New("无效的充值额度")
		}
		return int(topUp.QuotaAmount), nil
	}

	quota, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if clamp != nil {
		return 0, clamp
	}
	if quota <= 0 {
		return 0, errors.New("无效的充值额度")
	}
	return quota, nil
}

func enqueueTopUpCreditTx(tx *gorm.DB, topUp *TopUp, quota int) (*BillingAdjustmentOutbox, error) {
	if tx == nil || topUp == nil || topUp.UserId <= 0 {
		return nil, errors.New("充值用户不存在")
	}
	var user User
	if err := tx.Select("id").Where("id = ?", topUp.UserId).First(&user).Error; err != nil {
		return nil, errors.New("充值用户不存在")
	}
	return EnqueueBillingAdjustmentTx(tx, BillingAdjustmentSpec{
		RequestID: "topup:" + topUp.TradeNo,
		Phase:     BillingAdjustmentPhaseExternalCredit,
		Leg:       BillingAdjustmentLegWallet,
		UserID:    topUp.UserId,
		Delta:     int64(quota),
	}, false)
}

func processExternalCreditBestEffort(adjustment *BillingAdjustmentOutbox, source string) {
	if adjustment == nil {
		return
	}
	if err := ProcessBillingAdjustmentOutbox(adjustment.Id); err != nil {
		common.SysLog(fmt.Sprintf("%s credit queued for retry: outbox_id=%d err=%v", source, adjustment.Id, err))
	}
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	topUp := &TopUp{}
	var adjustment *BillingAdjustmentOutbox

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		var quotaErr error
		quota, quotaErr = common.QuotaFromFloatStrict(topUp.Money * common.QuotaPerUnit)
		if quotaErr != nil || quota <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		result := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("stripe_customer", customerId)
		if result.Error != nil {
			return result.Error
		}
		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quota)
		return enqueueErr
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	processExternalCreditBestEffort(adjustment, "stripe topup")

	if quota > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)
	}

	return nil
}

// RechargeEpay atomically completes an Epay order and records its durable
// wallet credit claim. The claim is processed after commit and remains in the
// outbox when Redis or quota headroom is temporarily unavailable.
func RechargeEpay(tradeNo string, actualPaymentMethod string, settlement TopUpSettlement, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	topUp := &TopUp{}
	quotaToAdd := 0
	var adjustment *BillingAdjustmentOutbox
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}
		if err := validateTopUpSettlement(topUp, settlement, false); err != nil {
			return err
		}

		var quotaErr error
		quotaToAdd, quotaErr = getTopUpQuotaToCredit(topUp)
		if quotaErr != nil {
			return quotaErr
		}

		if actualPaymentMethod != "" {
			topUp.PaymentMethod = actualPaymentMethod
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quotaToAdd)
		return enqueueErr
	})
	if err != nil {
		common.SysError("epay topup failed: " + err.Error())
		return err
	}

	processExternalCreditBestEffort(adjustment, "epay topup")
	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	}
	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var adjustment *BillingAdjustmentOutbox

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		userId = topUp.UserId
		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// New orders snapshot quota at creation. Legacy orders retain their
		// provider-specific calculation for backward compatibility.
		if topUp.QuotaAmount > 0 {
			var quotaErr error
			quotaToAdd, quotaErr = getTopUpQuotaToCredit(topUp)
			if quotaErr != nil {
				return quotaErr
			}
		} else if topUp.PaymentProvider == PaymentProviderStripe {
			var quotaErr error
			quotaToAdd, quotaErr = common.QuotaFromFloatStrict(topUp.Money * common.QuotaPerUnit)
			if quotaErr != nil {
				return quotaErr
			}
		} else {
			var quotaErr error
			quotaToAdd, quotaErr = getTopUpQuotaToCredit(topUp)
			if quotaErr != nil {
				return quotaErr
			}
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quotaToAdd)
		if enqueueErr != nil {
			return enqueueErr
		}
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}
	processExternalCreditBestEffort(adjustment, "manual topup")

	// 事务外记录日志，避免阻塞
	if quotaToAdd > 0 {
		RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	topUp := &TopUp{}
	var adjustment *BillingAdjustmentOutbox

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}
		if topUp.Amount <= 0 || topUp.Amount > int64(common.MaxQuota) {
			return errors.New("无效的充值额度")
		}
		quota = int(topUp.Amount)

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		var user User
		if err := tx.Where("id = ?", topUp.UserId).First(&user).Error; err != nil {
			return errors.New("充值用户不存在")
		}
		if customerEmail != "" && user.Email == "" {
			if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("email", customerEmail).Error; err != nil {
				return err
			}
		}

		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quota)
		return enqueueErr
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	processExternalCreditBestEffort(adjustment, "creem topup")

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, settlement TopUpSettlement, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}
	var adjustment *BillingAdjustmentOutbox

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}
		if err := validateTopUpSettlement(topUp, settlement, true); err != nil {
			return err
		}

		var quotaErr error
		quotaToAdd, quotaErr = getTopUpQuotaToCredit(topUp)
		if quotaErr != nil {
			return quotaErr
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quotaToAdd)
		return enqueueErr
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	processExternalCreditBestEffort(adjustment, "waffo topup")

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}
	var adjustment *BillingAdjustmentOutbox

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		var quotaErr error
		quotaToAdd, quotaErr = getTopUpQuotaToCredit(topUp)
		if quotaErr != nil {
			return quotaErr
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		var enqueueErr error
		adjustment, enqueueErr = enqueueTopUpCreditTx(tx, topUp, quotaToAdd)
		return enqueueErr
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	processExternalCreditBestEffort(adjustment, "waffo pancake topup")

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}
