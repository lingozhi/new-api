package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeForwardsGeometryOutsideVerifiedCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, appI18n.Init())
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	model.ClearChannelCacheForTest()
	t.Cleanup(func() {
		model.ClearChannelCacheForTest()
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	priority := int64(10)
	weight := uint(100)
	channel := &model.Channel{
		Id:       31,
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "verified-auto-image",
		Models:   "gpt-image-2",
		Group:    "gpt pro",
		Priority: &priority,
		Weight:   &weight,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: &dto.ImageRoutingConfig{
		Version: dto.ImageRoutingVersion1,
		Profiles: []dto.ImageRoutingProfile{
			{
				Model:               "gpt-image-2",
				Protocol:            dto.ImageRoutingProtocolImagesGenerations,
				UpstreamPath:        "/v1/images/generations",
				Operations:          []dto.ImageOperation{dto.ImageOperationGeneration},
				Sizes:               []string{"auto"},
				DefaultSize:         "auto",
				MaxOutputImages:     1,
				AllowedCombinations: []dto.ImageRoutingCombination{{Operation: dto.ImageOperationGeneration, Size: "auto"}},
				VerificationStatus:  dto.ImageRoutingVerificationProductionVerified,
			},
		},
	}})
	model.SetChannelCacheForTest(map[int]*model.Channel{31: channel}, map[string]map[string][]int{
		"gpt pro": {"gpt-image-2": {31}},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/jobs",
		strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cube","size":"1024x1024","n":1}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "gpt pro")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	Distribute()(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.False(t, ctx.IsAborted())
	selected, ok := common.GetContextKey(ctx, constant.ContextKeyChannelId)
	require.True(t, ok)
	assert.Equal(t, channel.Id, selected)
}
