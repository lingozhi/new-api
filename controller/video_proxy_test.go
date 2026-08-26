package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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
