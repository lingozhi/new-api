package router

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	newapii18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAutoDLAudioRouterTest(t *testing.T) (*gorm.DB, *model.User, *model.Token) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:router-autodl-audio-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.Ability{},
		&model.Task{},
		&model.Log{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.BillingAdjustmentOutbox{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousDataExportEnabled := common.DataExportEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	common.DataExportEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.ClearChannelCooldownsForTest()
	model.ClearChannelCacheForTest()
	service.InitHttpClient()
	t.Cleanup(func() {
		model.ClearChannelCooldownsForTest()
		model.ClearChannelCacheForTest()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.DataExportEnabled = previousDataExportEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		service.InitHttpClient()
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := &model.User{
		Username: "router-audio-user",
		Password: "password",
		Quota:    1_000_000,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "routerautodlaudiotoken",
		Name:        "router audio token",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 1_000_000,
	}
	require.NoError(t, db.Create(token).Error)
	return db, user, token
}

func TestIndexTTSAudioReplayRunsBeforeChannelDistribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, user, token := setupAutoDLAudioRouterTest(t)
	const idempotencyKey = "router-index-tts-replay"
	requestBody := []byte(`{"model":"indextts2-v1","input":"replay without selecting a channel","voice":"https://media.example.com/speaker.wav","response_format":"wav"}`)

	var request dto.AudioRequest
	require.NoError(t, common.Unmarshal(requestBody, &request))
	canonicalRequest, err := common.Marshal(request)
	require.NoError(t, err)
	clientRequestID := common.Sha256([]byte("autodl-audio-idempotency:v1:" + idempotencyKey))
	requestHash := common.Sha256(append([]byte("autodl-audio-request:v1:"), canonicalRequest...))
	task := &model.Task{
		TaskID:          "task_router_audio_replay",
		Platform:        constant.TaskPlatformAutoDL,
		UserId:          user.Id,
		ClientRequestID: &clientRequestID,
		Action:          constant.TaskActionAudioSpeech,
		Status:          model.TaskStatusReserving,
		SubmitTime:      time.Now().Unix(),
		Properties:      model.Properties{OriginModelName: constant.AutoDLModelIndexTTS2},
		PrivateData: model.TaskPrivateData{
			ClientRequestHash: requestHash,
			TokenId:           token.Id,
		},
	}
	require.NoError(t, db.Create(task).Error)
	token.Status = common.TokenStatusExhausted
	token.RemainQuota = 0
	require.NoError(t, db.Save(token).Error)

	// No channel or ability is seeded. The request can succeed only when replay
	// resolves and aborts the chain before strict quota auth and distribution.
	engine := gin.New()
	SetRelayRouter(engine)
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(string(requestBody)))
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	httpRequest.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httpRequest)

	assert.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	assert.Equal(t, "true", recorder.Header().Get("Idempotency-Replayed"))
	assert.Equal(t, task.TaskID, recorder.Header().Get("X-New-Api-Task-ID"))
	assert.Equal(t, "/v1/audio/speech/"+task.TaskID, recorder.Header().Get("Location"))
	assert.Contains(t, recorder.Body.String(), task.TaskID)

	otherToken := &model.Token{
		UserId:      user.Id,
		Key:         "routerautodlaudioothertoken",
		Name:        "router audio other token",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 1_000_000,
	}
	require.NoError(t, db.Create(otherToken).Error)
	unauthorizedRequest := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(string(requestBody)))
	unauthorizedRequest.Header.Set("Content-Type", "application/json")
	unauthorizedRequest.Header.Set("Authorization", "Bearer sk-"+otherToken.Key)
	unauthorizedRequest.Header.Set("Idempotency-Key", idempotencyKey)
	unauthorizedRecorder := httptest.NewRecorder()

	engine.ServeHTTP(unauthorizedRecorder, unauthorizedRequest)

	assert.Equal(t, http.StatusNotFound, unauthorizedRecorder.Code, unauthorizedRecorder.Body.String())
	assert.Empty(t, unauthorizedRecorder.Header().Get("Idempotency-Replayed"))
	assert.NotContains(t, unauthorizedRecorder.Body.String(), task.TaskID)
}

func TestIndexTTSAudioRecoveryRejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, token := setupAutoDLAudioRouterTest(t)
	token.ExpiredTime = time.Now().Add(-time.Minute).Unix()
	require.NoError(t, db.Save(token).Error)

	engine := gin.New()
	SetRelayRouter(engine)
	httpRequest := httptest.NewRequest(http.MethodGet, "/v1/audio/speech/task_expired_token", nil)
	httpRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httpRequest)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"error"`)
}

func TestOrdinaryTTSAudioRouteStillUsesSynchronousRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, token := setupAutoDLAudioRouterTest(t)
	previousModelPrices := ratio_setting.ModelPrice2JSONString()
	modelPrices := ratio_setting.GetModelPriceMap()
	modelPrices["gpt-4o-mini-tts"] = 0.3
	encodedModelPrices, err := common.Marshal(modelPrices)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(encodedModelPrices)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousModelPrices)) })
	upstreamCalled := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v1/audio/speech", request.URL.Path)
		assert.Equal(t, "Bearer upstream-openai-key", request.Header.Get("Authorization"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		upstreamCalled <- body
		writer.Header().Set("Content-Type", "audio/mpeg")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("legacy-synchronous-audio"))
	}))
	defer upstream.Close()

	priority := int64(10)
	weight := uint(100)
	autoBan := 0
	channel := &model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Key:         "upstream-openai-key",
		Name:        "ordinary TTS channel",
		Status:      common.ChannelStatusEnabled,
		BaseURL:     &upstream.URL,
		Models:      "gpt-4o-mini-tts",
		Group:       "default",
		Priority:    &priority,
		Weight:      &weight,
		AutoBan:     &autoBan,
		CreatedTime: time.Now().Unix(),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-4o-mini-tts",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	common.MemoryCacheEnabled = true
	model.SetChannelCacheForTest(map[int]*model.Channel{channel.Id: channel}, map[string]map[string][]int{
		"default": {"gpt-4o-mini-tts": {channel.Id}},
	})

	engine := gin.New()
	SetRelayRouter(engine)
	requestBody := `{"model":"gpt-4o-mini-tts","input":"ordinary speech","voice":"alloy","response_format":"mp3"}`
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(requestBody))
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	httpRequest.Header.Set("Idempotency-Key", strings.Repeat("ordinary-tts-key", 20))
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httpRequest)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, "legacy-synchronous-audio", recorder.Body.String())
	select {
	case upstreamBody := <-upstreamCalled:
		var forwarded dto.AudioRequest
		require.NoError(t, common.Unmarshal(upstreamBody, &forwarded))
		assert.Equal(t, "gpt-4o-mini-tts", forwarded.Model)
		assert.Equal(t, "ordinary speech", forwarded.Input)
		assert.Equal(t, "alloy", forwarded.Voice)
	case <-time.After(time.Second):
		t.Fatal("ordinary TTS request never reached the synchronous upstream relay")
	}
	var taskCount int64
	require.NoError(t, db.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount, "ordinary TTS must not create an AutoDL durable task")
	require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, time.Second, time.Millisecond)
}

func TestIndexTTSAudioNonJSONCannotBypassIdempotency(t *testing.T) {
	require.NoError(t, newapii18n.Init())
	tests := []struct {
		name       string
		body       func(t *testing.T) (io.Reader, string)
		withKey    bool
		wantStatus int
		wantCode   string
	}{
		{
			name: "url encoded without key",
			body: func(_ *testing.T) (io.Reader, string) {
				return strings.NewReader("model=indextts2-v1&input=hello&voice=https%3A%2F%2Fmedia.example.com%2Fvoice.wav"), "application/x-www-form-urlencoded"
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "idempotency_key_required",
		},
		{
			name: "multipart without key",
			body: func(t *testing.T) (io.Reader, string) {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("model", constant.AutoDLModelIndexTTS2))
				require.NoError(t, writer.WriteField("input", "hello"))
				require.NoError(t, writer.WriteField("voice", "https://media.example.com/voice.wav"))
				require.NoError(t, writer.Close())
				return bytes.NewReader(body.Bytes()), writer.FormDataContentType()
			},
			// Speech multipart has historically ignored its model field and
			// falls back to tts-1. The important invariant is that it cannot
			// select AutoDL, create a task, or consume quota.
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "model_not_found",
		},
		{
			name: "url encoded with key",
			body: func(_ *testing.T) (io.Reader, string) {
				return strings.NewReader("model=indextts2-v1&input=hello&voice=https%3A%2F%2Fmedia.example.com%2Fvoice.wav"), "application/x-www-form-urlencoded"
			},
			withKey:    true,
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db, user, token := setupAutoDLAudioRouterTest(t)
			priority := int64(10)
			weight := uint(100)
			autoBan := 0
			baseURL := "https://autodl.art"
			channel := &model.Channel{
				Type:        constant.ChannelTypeAutoDL,
				Key:         "autodl-test-key",
				Name:        "AutoDL IndexTTS2 test channel",
				Status:      common.ChannelStatusEnabled,
				BaseURL:     &baseURL,
				Models:      constant.AutoDLModelIndexTTS2,
				Group:       "default",
				Priority:    &priority,
				Weight:      &weight,
				AutoBan:     &autoBan,
				CreatedTime: time.Now().Unix(),
			}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{
				Group:     "default",
				Model:     constant.AutoDLModelIndexTTS2,
				ChannelId: channel.Id,
				Enabled:   true,
				Priority:  &priority,
				Weight:    weight,
			}).Error)
			common.MemoryCacheEnabled = true
			model.SetChannelCacheForTest(map[int]*model.Channel{channel.Id: channel}, map[string]map[string][]int{
				"default": {constant.AutoDLModelIndexTTS2: {channel.Id}},
			})

			var initialUserQuota int
			require.NoError(t, db.Model(&model.User{}).Select("quota").Where("id = ?", user.Id).Scan(&initialUserQuota).Error)
			var initialTokenQuota int
			require.NoError(t, db.Model(&model.Token{}).Select("remain_quota").Where("id = ?", token.Id).Scan(&initialTokenQuota).Error)
			if test.withKey {
				replayRequest := &dto.AudioRequest{
					Model: constant.AutoDLModelIndexTTS2,
					Input: "hello",
					Voice: "https://media.example.com/voice.wav",
				}
				canonicalRequest, err := common.Marshal(replayRequest)
				require.NoError(t, err)
				clientRequestID := common.Sha256([]byte("autodl-audio-idempotency:v1:non-json-must-not-submit"))
				requestHash := common.Sha256(append([]byte("autodl-audio-request:v1:"), canonicalRequest...))
				require.NoError(t, db.Create(&model.Task{
					TaskID:          "task_existing_non_json_replay",
					Platform:        constant.TaskPlatformAutoDL,
					UserId:          user.Id,
					ClientRequestID: &clientRequestID,
					Action:          constant.TaskActionAudioSpeech,
					Status:          model.TaskStatusCheckpointPending,
					SubmitTime:      time.Now().Unix(),
					Properties:      model.Properties{OriginModelName: constant.AutoDLModelIndexTTS2},
					PrivateData: model.TaskPrivateData{
						ClientRequestHash: requestHash,
						TokenId:           token.Id,
					},
				}).Error)
			}
			var initialTaskCount int64
			require.NoError(t, db.Model(&model.Task{}).Count(&initialTaskCount).Error)

			requestBody, contentType := test.body(t)
			engine := gin.New()
			SetRelayRouter(engine)
			request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", requestBody)
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			if test.withKey {
				request.Header.Set("Idempotency-Key", "non-json-must-not-submit")
			}
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			assert.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), test.wantCode)
			var taskCount int64
			require.NoError(t, db.Model(&model.Task{}).Count(&taskCount).Error)
			assert.Equal(t, initialTaskCount, taskCount)
			assert.Empty(t, recorder.Header().Get("Idempotency-Replayed"))
			assert.Empty(t, recorder.Header().Get("X-New-Api-Task-ID"))
			var finalUserQuota int
			require.NoError(t, db.Model(&model.User{}).Select("quota").Where("id = ?", user.Id).Scan(&finalUserQuota).Error)
			var finalTokenQuota int
			require.NoError(t, db.Model(&model.Token{}).Select("remain_quota").Where("id = ?", token.Id).Scan(&finalTokenQuota).Error)
			assert.Equal(t, initialUserQuota, finalUserQuota)
			assert.Equal(t, initialTokenQuota, finalTokenQuota)
		})
	}
}
