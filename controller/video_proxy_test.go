package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoProxyRejectsAutoDLAudioTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	task := &model.Task{
		TaskID:     "task_audio_not_video",
		Platform:   constant.TaskPlatformAutoDL,
		UserId:     91,
		Action:     constant.TaskActionAudioSpeech,
		Status:     model.TaskStatusSuccess,
		SubmitTime: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://media.example.com/generated.wav?signature=secret",
		},
	}
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID+"/content", nil)
	context.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	context.Set("id", task.UserId)

	VideoProxy(context)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), task.PrivateData.ResultURL)
}

func TestVideoProxyResumesNewVideoTasksWithoutChangingLegacyDownloads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	fetch.EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = previousDB, previousCache
		*fetch = previousFetch
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	for _, tc := range []struct {
		name, provider, rangeHeader    string
		upstreamStatus, expectedStatus int
		pending                        bool
	}{
		{"wan partial", "wan-unified", "bytes=3-", 206, 206, false},
		{"seedance partial", "lxmone-seedance", "bytes=3-", 206, 206, false},
		{"range unsatisfiable", "wan-unified", "bytes=99-", 416, 416, false},
		{"if range fallback", "wan-unified", "bytes=3-", 200, 200, false},
		{"full download", "wan-unified", "", 200, 200, false},
		{"unsolicited partial", "wan-unified", "", 206, 502, false},
		{"legacy range unchanged", "", "bytes=3-", 200, 200, false},
		{"wan pending", "wan-unified", "", 0, 409, true},
		{"legacy pending unchanged", "", "", 0, 400, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "/v1/videos/upstream/content", r.URL.Path)
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				if tc.provider != "" && tc.rangeHeader != "" {
					assert.Equal(t, tc.rangeHeader, r.Header.Get("Range"))
					assert.Equal(t, `"version-1"`, r.Header.Get("If-Range"))
				} else {
					assert.Empty(t, r.Header.Get("Range"))
					assert.Empty(t, r.Header.Get("If-Range"))
				}
				w.Header().Set("Content-Type", "video/mp4")
				w.Header().Set("ETag", `"version-1"`)
				if tc.upstreamStatus == 416 {
					w.Header().Set("Content-Range", "bytes */6")
				}
				if tc.upstreamStatus == 206 {
					w.Header().Set("Content-Range", "bytes 3-5/6")
				}
				w.WriteHeader(tc.upstreamStatus)
				if tc.upstreamStatus == 206 {
					_, _ = w.Write([]byte("def"))
				} else {
					_, _ = w.Write([]byte("abcdef"))
				}
			}))
			defer upstream.Close()
			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "test-key", BaseURL: &upstream.URL}
			require.NoError(t, db.Create(&channel).Error)
			task := model.Task{TaskID: "task_" + tc.name, UserId: 91, ChannelId: channel.Id, Platform: "sora", Status: model.TaskStatusSuccess}
			task.PrivateData.UpstreamTaskID = "upstream"
			if tc.pending {
				task.Status = model.TaskStatusInProgress
			}
			if tc.provider != "" {
				task.Properties.Video = &relaycommon.TaskVideoProperties{Provider: tc.provider}
			}
			require.NoError(t, db.Create(&task).Error)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/test/content", nil)
			c.Request.Header.Set("Range", tc.rangeHeader)
			c.Request.Header.Set("If-Range", `"version-1"`)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 91)
			VideoProxy(c)
			assert.Equal(t, tc.expectedStatus, recorder.Code)
			if tc.pending {
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, 1, calls)
			}
			if recorder.Code == 206 {
				assert.Equal(t, "def", recorder.Body.String())
				assert.Equal(t, "bytes 3-5/6", recorder.Header().Get("Content-Range"))
			}
			if recorder.Code == 200 {
				assert.Equal(t, "abcdef", recorder.Body.String())
			}
			if recorder.Code == 416 {
				assert.Equal(t, "bytes */6", recorder.Header().Get("Content-Range"))
			}
		})
	}
}
