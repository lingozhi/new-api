package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestMiniMaxVideoQueryAllowsExhaustedTokenAfterPaidSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}))

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

func TestMiniMaxVideoCreateReplaysIdempotentTaskBeforeDistribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}))

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

	user := &model.User{Username: "minimax-idempotency-user", Password: "password", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "minimaxidempotencytoken", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
	require.NoError(t, db.Create(token).Error)

	requestBody := `{"model":"MiniMax-H3","content":[{"type":"text","text":"a lighthouse"}],"resolution":"768P","duration":4,"ratio":"16:9"}`
	var requestDTO dto.MiniMaxVideoGenerationV2Request
	require.NoError(t, common.Unmarshal([]byte(requestBody), &requestDTO))
	canonicalRequest, err := common.Marshal(requestDTO)
	require.NoError(t, err)
	idempotencyKey := "minimax-video-create-replay"
	clientRequestID := common.Sha256([]byte("autodl-video-idempotency:v1:" + idempotencyKey))
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:          "task_minimax_idempotency_replay",
		Platform:        constant.TaskPlatformAutoDL,
		UserId:          user.Id,
		ClientRequestID: &clientRequestID,
		Status:          model.TaskStatusCheckpointPending,
		SubmitTime:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
		PrivateData: model.TaskPrivateData{
			ClientRequestHash: common.Sha256(append([]byte("autodl-video-request:v1:"), canonicalRequest...)),
		},
	}
	require.NoError(t, db.Create(task).Error)

	engine := gin.New()
	SetVideoRouter(engine)
	request := httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, "true", recorder.Header().Get("Idempotency-Replayed"))
	assert.Equal(t, "2", recorder.Header().Get("Retry-After"))
	assert.JSONEq(t, `{"task_id":"task_minimax_idempotency_replay"}`, recorder.Body.String())

	conflictingBody := strings.Replace(requestBody, "a lighthouse", "a mountain", 1)
	conflictingRequest := httptest.NewRequest(http.MethodPost, "/v2/video_generation", strings.NewReader(conflictingBody))
	conflictingRequest.Header.Set("Content-Type", "application/json")
	conflictingRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	conflictingRequest.Header.Set("Idempotency-Key", idempotencyKey)
	conflictingRecorder := httptest.NewRecorder()
	engine.ServeHTTP(conflictingRecorder, conflictingRequest)

	assert.Equal(t, http.StatusConflict, conflictingRecorder.Code, conflictingRecorder.Body.String())
}
