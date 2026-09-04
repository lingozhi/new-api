package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageChannelTestUsesGenerationEndpointAndProviderDefaultSize(t *testing.T) {
	user, _, channel := setupRelayAsyncImagePricingTest(t, `{}`)
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
	oldPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-2":0.045}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices)) })
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		var request dto.ImageRequest
		if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "2560x1440", request.Size)
		assert.Equal(t, "gpt-image-2", request.Model)
		assert.Equal(t, uint(1), *request.N)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.com/image.png"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()
	channel.BaseURL = &upstream.URL
	channel.Models = "gpt-image-2"
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageRouting: &dto.ImageRoutingConfig{
		Version: 1, Profiles: []dto.ImageRoutingProfile{{Model: "gpt-image-2", Protocol: dto.ImageRoutingProtocolImagesGenerations,
			UpstreamPath: "/v1/images/generations", Operations: []dto.ImageOperation{dto.ImageOperationGeneration},
			Sizes: []string{"2560x1440", "3840x2160"}, DefaultSize: "2560x1440",
		}},
	}})
	for _, endpoint := range []string{"image-generation", ""} {
		result := testChannel(context.Background(), channel, user.Id, "gpt-image-2", endpoint, false)
		assert.NoError(t, result.localErr)
		assert.Nil(t, result.newAPIError)
	}
}

func TestLocalBillingFailureDoesNotRetryOrCooldownChannel(t *testing.T) {
	oldLogging := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = oldLogging })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	channel := *types.NewChannelError(987654, constant.ChannelTypeOpenAI, "billing-local", false, "test", true)
	for _, code := range []types.ErrorCode{types.ErrorCodePreConsumeTokenQuotaFailed, types.ErrorCodeUpdateDataError, types.ErrorCodeInsufficientUserQuota} {
		apiErr := types.NewErrorWithStatusCode(errors.New("local accounting failed"), code, http.StatusForbidden)
		assert.False(t, shouldRetry(c, apiErr, 3))
		assert.False(t, isRetryableChannelError(c, apiErr))
		processChannelError(c, channel, apiErr)
		_, _, cooling := model.GetChannelCooldown(channel.ChannelId)
		assert.False(t, cooling)
	}
	upstreamErr := types.NewErrorWithStatusCode(errors.New("provider unavailable"), types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
	assert.False(t, types.IsLocalRelayError(upstreamErr))
	assert.True(t, shouldRetry(c, upstreamErr, 3))
}
