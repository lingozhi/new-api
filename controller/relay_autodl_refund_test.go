package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const autoDLRefundTestInitialQuota = 1_000_000

func setupAutoDLRefundControllerTest(t *testing.T, upstreamStatus int) (*model.User, *model.Token, *model.Channel) {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Deliberately advertise a longer body than is sent. Reading it produces
		// unexpected EOF, matching a proxy/upstream reset after response headers.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(upstreamStatus)
		_, _ = w.Write([]byte(`{`))
	}))
	t.Cleanup(server.Close)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:autodl-refund-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.Log{},
		&model.Task{},
		&model.TaskWebhook{},
		&model.ImageBillingReservation{},
		&model.BillingAdjustmentOutbox{},
		&model.ImageTaskBillingLogOutbox{},
		&model.ImageTaskBillingLogReceipt{},
		&model.ImageInputCleanup{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousDataExportEnabled := common.DataExportEnabled
	previousTLSInsecureSkipVerify := common.TLSInsecureSkipVerify
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	common.DataExportEnabled = false
	common.TLSInsecureSkipVerify = true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"autodl-refund-test":1}`))
	service.InitHttpClient()
	model.ClearChannelCooldownsForTest()
	t.Cleanup(func() {
		model.ClearChannelCooldownsForTest()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.DataExportEnabled = previousDataExportEnabled
		common.TLSInsecureSkipVerify = previousTLSInsecureSkipVerify
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		service.InitHttpClient()
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := &model.User{
		Username: "autodl-refund-user",
		Password: "password",
		Quota:    autoDLRefundTestInitialQuota,
		Status:   common.UserStatusEnabled,
		Group:    "autodl-refund-test",
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "autodl-refund-token",
		Name:        "AutoDL refund test token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: autoDLRefundTestInitialQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)
	autoBan := 0
	channel := &model.Channel{
		Type:        constant.ChannelTypeAutoDL,
		Key:         "autodl-upstream-key",
		Name:        "AutoDL refund test channel",
		Status:      common.ChannelStatusEnabled,
		CreatedTime: 1_700_000_000,
		BaseURL:     &server.URL,
		Models:      "MiniMax-H3",
		Group:       "autodl-refund-test",
		AutoBan:     &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return user, token, channel
}

func relayAutoDLRefundRequest(t *testing.T, user *model.User, token *model.Token, channel *model.Channel) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"A paper boat at sunset"}],
		"resolution":"768P",
		"duration":4,
		"ratio":"16:9"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	c.Set(common.RequestIdKey, "request-autodl-refund")
	c.Set("token_name", token.Name)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "MiniMax-H3")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, false)
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{
		BillingPreference:     "wallet_only",
		QuotaWarningThreshold: -1,
	})
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, *channel.BaseURL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{})
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{})
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, map[string]any{})
	c.Set("auto_ban", false)

	RelayTask(c)
	return recorder
}

func TestRelayTaskAutoDLUpstreamFailureUsesStandardRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		upstreamStatus int
	}{
		{name: "400 truncated response", upstreamStatus: http.StatusBadRequest},
		{name: "408 truncated response", upstreamStatus: http.StatusRequestTimeout},
		{name: "429 truncated response", upstreamStatus: http.StatusTooManyRequests},
		{name: "499 truncated response", upstreamStatus: 499},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user, token, channel := setupAutoDLRefundControllerTest(t, test.upstreamStatus)
			recorder := relayAutoDLRefundRequest(t, user, token, channel)

			require.NoError(t, model.DB.First(user, user.Id).Error)
			require.NoError(t, model.DB.First(token, token.Id).Error)
			require.NoError(t, model.DB.First(channel, channel.Id).Error)

			assert.Equal(t, test.upstreamStatus, recorder.Code, recorder.Body.String())
			assert.Equal(t, autoDLRefundTestInitialQuota, user.Quota)
			assert.Equal(t, autoDLRefundTestInitialQuota, token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
			assert.Zero(t, channel.UsedQuota)
			var taskCount int64
			require.NoError(t, model.DB.Model(&model.Task{}).Where("platform = ?", constant.TaskPlatformAutoDL).Count(&taskCount).Error)
			assert.Zero(t, taskCount)
			var response dto.MiniMaxAPIErrorResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, fmt.Sprintf("%d", test.upstreamStatus), response.Error.HTTPCode)
		})
	}
}
