package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestArgolinkUploadTicketRelay(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/media/uploads", r.URL.Path)
		assert.Equal(t, "Bearer selected-key", r.Header.Get("Authorization"))
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		assert.Equal(t, constant.ArgolinkSeedance25Model, payload["model"])
		assert.Equal(t, float64(123), payload["size_bytes"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"upload_url":"https://storage.example/upload","media_url":"https://storage.example/media"}`))
	}))
	defer upstream.Close()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = oldDB, oldCache
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	ch := &model.Channel{Type: constant.ChannelTypeOpenAI, BaseURL: &upstream.URL, Key: "not-selected"}
	require.NoError(t, db.Create(ch).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(`{"model":"seedance-2.5","type":"image","content_type":"image/png","size_bytes":123}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "selected-key")
	ArgolinkMediaUpload(c)
	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.JSONEq(t, `{"upload_url":"https://storage.example/upload","media_url":"https://storage.example/media"}`, recorder.Body.String())
}
