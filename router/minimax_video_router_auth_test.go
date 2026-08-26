package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMiniMaxVideoQueryAllowsExhaustedTokenAfterPaidSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.TaskWebhook{}))

	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := &model.User{
		Username: "minimax-query-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "minimaxexhaustedtoken",
		Status:      common.TokenStatusExhausted,
		ExpiredTime: -1,
		RemainQuota: 0,
	}
	require.NoError(t, db.Create(token).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Task{
		TaskID:     "task_minimax_paid",
		Platform:   constant.TaskPlatform("60"),
		UserId:     user.Id,
		Action:     constant.TaskActionVideoGenerationV2,
		Status:     model.TaskStatusQueued,
		CreatedAt:  now,
		UpdatedAt:  now,
		SubmitTime: now,
		Properties: model.Properties{OriginModelName: "MiniMax-H3"},
	}).Error)

	engine := gin.New()
	SetVideoRouter(engine)
	request := httptest.NewRequest(http.MethodGet, "/v2/query/video_generation/task_minimax_paid", nil)
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	expected, err := common.Marshal(map[string]any{
		"task": map[string]any{
			"id":         "task_minimax_paid",
			"model":      "MiniMax-H3",
			"status":     "queued",
			"created_at": now,
			"updated_at": now,
			"task_type":  "generation",
			"modality":   "video",
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), recorder.Body.String())
}
