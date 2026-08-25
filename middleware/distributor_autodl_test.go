package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelSupportsMiniMaxV2OnlyForAutoDL(t *testing.T) {
	requestPath := "/v2/video_generation"
	requestModel := "MiniMax-H3"

	assert.True(t, channelSupportsRequestPath(&model.Channel{Type: constant.ChannelTypeAutoDL}, requestPath, requestModel))
	assert.False(t, channelSupportsRequestPath(&model.Channel{Type: constant.ChannelTypeMiniMax}, requestPath, requestModel))
	assert.False(t, channelSupportsRequestPath(&model.Channel{Type: constant.ChannelTypeOpenAI}, requestPath, requestModel))
	assert.False(t, channelSupportsRequestPath(&model.Channel{Type: constant.ChannelTypeAutoDL}, "/v1/videos", requestModel))
	assert.False(t, channelSupportsRequestPath(&model.Channel{Type: constant.ChannelTypeAutoDL}, "/v1/video/generations", requestModel))
}

func TestDistributorRejectsFixedNonAutoDLChannelForMiniMaxV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Channel{
		Id:     621,
		Type:   constant.ChannelTypeMiniMax,
		Name:   "wrong-minimax-channel",
		Key:    "provider-key",
		Status: common.ChannelStatusEnabled,
		Models: "MiniMax-H3",
		Group:  "default",
	}).Error)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "621")
		c.Set(common.RequestIdKey, "req_fixed_channel")
		c.Next()
	})
	engine.Use(Distribute())
	engine.POST("/v2/video_generation", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(`{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"test"}],
		"resolution":"768P",
		"duration":5,
		"ratio":"16:9"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	var response dto.MiniMaxAPIErrorResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "bad_request_error", response.Error.Type)
	assert.Contains(t, response.Error.Message, "does not support request path")
}
