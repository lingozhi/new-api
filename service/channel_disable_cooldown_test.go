package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

func TestShouldDisableChannelIgnoresCooldownBalanceError(t *testing.T) {
	oldAutomaticDisableChannelEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutomaticDisableChannelEnabled
	})

	err := types.NewErrorWithStatusCode(errors.New("Insufficient account balance"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)

	if ShouldDisableChannel(err) {
		t.Fatalf("expected balance error to cooldown without permanent auto-disable")
	}
}

func TestShouldDisableChannelIgnoresUpstream429(t *testing.T) {
	oldAutomaticDisableChannelEnabled := common.AutomaticDisableChannelEnabled
	oldDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusTooManyRequests, End: http.StatusTooManyRequests}}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutomaticDisableChannelEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldDisableRanges
	})

	err := types.NewErrorWithStatusCode(errors.New("upstream rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	err.UpstreamStatusCode = http.StatusTooManyRequests

	assert.False(t, ShouldDisableChannel(err), "upstream 429 should use a temporary cooldown instead of permanently disabling the channel")
}

func TestShouldDisableChannelIgnoresPreCommitStreamCapacity(t *testing.T) {
	oldAutomaticDisableChannelEnabled := common.AutomaticDisableChannelEnabled
	oldDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusServiceUnavailable, End: http.StatusServiceUnavailable}}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutomaticDisableChannelEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldDisableRanges
	})

	err := types.NewOpenAIError(
		errors.New("We're currently experiencing high demand, which may cause temporary errors."),
		types.ErrorCode("server_error"),
		http.StatusServiceUnavailable,
		types.ErrOptionWithPreCommitStreamCapacity(),
	)

	assert.False(t, ShouldDisableChannel(err), "a transient pre-commit capacity signal must use stream-quality cooldowns, not permanent auto-disable")
}

func TestQuarantineAsyncImageChannelBlocksCoolingFallbackImmediately(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	model.ClearChannelCacheForTest()
	t.Cleanup(func() {
		model.CooldownChannel(99117, "test cleanup", -time.Second)
		model.ClearChannelCacheForTest()
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})
	priority := int64(10)
	weight := uint(100)
	channel := &model.Channel{Id: 99117, Type: 1, Key: "test-key", Status: common.ChannelStatusEnabled, Name: "image failover", Priority: &priority, Weight: &weight}
	model.SetChannelCacheForTest(map[int]*model.Channel{channel.Id: channel}, map[string]map[string][]int{
		"default": {"gpt-image-2": {channel.Id}},
	})
	apiErr := types.NewErrorWithStatusCode(errors.New("provider unavailable"), types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
	apiErr.UpstreamStatusCode = http.StatusServiceUnavailable

	QuarantineAsyncImageChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, false, channel.Key, false), apiErr)

	assert.True(t, model.IsChannelCoolingDown(channel.Id))
	selected, err := model.GetRandomSatisfiedChannelWithOptions("default", "gpt-image-2", 0, model.ChannelSelectionOptions{AllowCoolingFallback: true})
	assert.NoError(t, err)
	assert.Nil(t, selected, "a failed image channel must not return as a cooling fallback")
}

func TestShouldQuarantineAsyncImageChannelClassifiesProviderAndClientFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorCode  types.ErrorCode
		message    string
		want       bool
	}{
		{name: "caller bad request", statusCode: http.StatusBadRequest, errorCode: types.ErrorCodeInvalidRequest, message: "invalid size", want: false},
		{name: "caller unprocessable request", statusCode: http.StatusUnprocessableEntity, errorCode: types.ErrorCodeInvalidRequest, message: "invalid prompt", want: false},
		{name: "content policy rejection", statusCode: http.StatusBadRequest, errorCode: types.ErrorCodePromptBlocked, message: "prompt blocked", want: false},
		{name: "authentication failure", statusCode: http.StatusUnauthorized, errorCode: types.ErrorCodeAccessDenied, message: "invalid provider key", want: true},
		{name: "access failure", statusCode: http.StatusForbidden, errorCode: types.ErrorCodeAccessDenied, message: "provider denied access", want: true},
		{name: "accepted task disappeared", statusCode: http.StatusNotFound, errorCode: types.ErrorCodeBadResponseStatusCode, message: "task not found", want: true},
		{name: "rate limit", statusCode: http.StatusTooManyRequests, errorCode: types.ErrorCodeBadResponseStatusCode, message: "rate limited", want: true},
		{name: "provider unavailable", statusCode: http.StatusServiceUnavailable, errorCode: types.ErrorCodeBadResponseStatusCode, message: "provider unavailable", want: true},
		{name: "capability gap", statusCode: http.StatusBadRequest, errorCode: types.ErrorCodeBadResponseStatusCode, message: "image generation is not enabled for this group", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiErr := types.NewErrorWithStatusCode(errors.New(test.message), test.errorCode, test.statusCode, types.ErrOptionWithSkipRetry())
			apiErr.UpstreamStatusCode = test.statusCode
			assert.Equal(t, test.want, ShouldQuarantineAsyncImageChannel(apiErr))
		})
	}
}

func TestShouldCooldownChannelForUpstreamErrorCoolsMalformedResponses(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("API returned an empty or malformed response (HTTP 200)"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)

	if !ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected malformed upstream response to cooldown")
	}
}

func TestShouldCooldownChannelForUpstreamErrorCoolsSkipRetryMalformedResponses(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("API returned an empty or malformed response (HTTP 200)"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())

	if !ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected malformed upstream response to cooldown even when retry is skipped")
	}
}

func TestShouldCooldownChannelForUpstreamErrorCoolsBadGateway(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{Message: "openai_error", Type: "openai_error", Code: "openai_error"}, http.StatusBadGateway)

	if !ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected upstream 502 to cooldown")
	}
}

func TestShouldCooldownChannelForUpstreamErrorUsesUnmappedUpstreamStatus(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("provider overloaded"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
	err.UpstreamStatusCode = http.StatusServiceUnavailable

	assert.True(t, ShouldCooldownChannelForUpstreamError(err), "expected an upstream 503 to cooldown after the client status is remapped")
}

func TestShouldCooldownChannelForUpstreamErrorCoolsImageGenerationCapabilityGap(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("Image generation is not enabled for this group"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)

	if !ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected per-channel capability gap (image generation disabled) to cooldown despite being 4xx")
	}
}

func TestShouldCooldownChannelForUpstreamErrorIgnoresClientErrors(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())

	if ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected client validation error to avoid cooldown")
	}
}

func TestShouldCooldownChannelForUpstreamErrorIgnoresAuthErrors(t *testing.T) {
	err := types.NewErrorWithStatusCode(errors.New("invalid token"), types.ErrorCodeAccessDenied, http.StatusUnauthorized)

	if ShouldCooldownChannelForUpstreamError(err) {
		t.Fatalf("expected auth error to avoid cooldown")
	}
}
