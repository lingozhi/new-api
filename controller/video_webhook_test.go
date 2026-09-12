package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoWebhookReachesDurableOutboxOnlyAfterAcceptanceAndCompletion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskWebhook{}))
	previous := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previous
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	for _, name := range []string{"wan3.0", "seedance-2"} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			requestJSON := `{"model":"` + name + `","prompt":"test","webhook_url":"https://8.8.8.8/hook","webhook_secret":"test-signing-secret"}`
			if name == "wan3.0" {
				requestJSON = strings.Replace(requestJSON, `"prompt":"test"`, `"input":{"prompt":"test"}`, 1)
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(requestJSON))
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{OriginModelName: name, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://lxmone.xyz", UpstreamModelName: name}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			info.ChannelOtherSettings.LxmoneSeedanceResolutionRatios = map[string]map[string]float64{"seedance-2-pro": {"720p": 1}}
			a := &sora.TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			hook := taskWebhookFromContext(c, model.TaskWebhookStatusPrepared)
			require.NotNil(t, hook)
			task := &model.Task{TaskID: "task_" + name, Platform: "3", Status: model.TaskStatusInProgress, Properties: model.Properties{OriginModelName: name, Video: info.TaskRelayInfo.Video}}
			require.NoError(t, model.InsertTaskWithWebhook(task, hook))
			due, err := model.FindDueTaskWebhooks(common.GetTimestamp(), 10)
			require.NoError(t, err)
			assert.Empty(t, due)
			require.NoError(t, model.ArmPreparedTaskWebhook(task.TaskID))
			due, err = model.FindDueTaskWebhooks(common.GetTimestamp(), 10)
			require.NoError(t, err)
			assert.Empty(t, due)
			require.NoError(t, db.Model(task).Update("status", model.TaskStatusSuccess).Error)
			due, err = model.FindDueTaskWebhooks(common.GetTimestamp(), 10)
			require.NoError(t, err)
			require.Len(t, due, 1)
			assert.Equal(t, task.TaskID, due[0].DeliveryID())
			require.NoError(t, db.Delete(&model.TaskWebhook{}, "task_id = ?", task.TaskID).Error)
		})
	}
}
