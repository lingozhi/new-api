package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAppendPromptCacheAdminInfoRecordsRawResponsesUsage(t *testing.T) {
	rawUsage := &dto.Usage{
		InputTokens:        28000,
		InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 16384},
	}
	bridgedUsage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 999},
		BillingUsage:        dto.NewOpenAIResponsesBillingUsage(rawUsage),
	}
	other := map[string]interface{}{"admin_info": map[string]interface{}{}}
	info := &relaycommon.RelayInfo{
		PromptCacheKeySource:       "metadata_user_id",
		PromptCacheBreakpointCount: 2,
	}

	appendPromptCacheAdminInfo(other, info, bridgedUsage)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "metadata_user_id", adminInfo["prompt_cache_key_source"])
	require.Equal(t, 2, adminInfo["prompt_cache_breakpoint_count"])
	require.Equal(t, 16384, adminInfo["upstream_input_cached_tokens"])
}

func TestAppendPromptCacheAdminInfoAlwaysRecordsDefaults(t *testing.T) {
	other := map[string]interface{}{}

	appendPromptCacheAdminInfo(other, &relaycommon.RelayInfo{PromptCacheKeySource: "none"}, nil)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "none", adminInfo["prompt_cache_key_source"])
	require.Equal(t, 0, adminInfo["prompt_cache_breakpoint_count"])
	require.Equal(t, 0, adminInfo["upstream_input_cached_tokens"])
}

func TestGenerateTextOtherInfoSeparatesRequestAndFinalAttemptFRT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("use_channel", []string{"68", "91"})

	requestStart := time.Unix(1_700_000_000, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         requestStart,
		FirstResponseTime: requestStart.Add(20 * time.Second),
		IsStream:          true,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}
	attemptStart := info.BeginChannelAttempt()
	info.StreamStatus = &relaycommon.StreamStatus{
		StartedAt:   attemptStart.Add(500 * time.Millisecond),
		FirstDataAt: attemptStart.Add(1500 * time.Millisecond),
		LastDataAt:  attemptStart.Add(2 * time.Second),
		EndedAt:     attemptStart.Add(2 * time.Second),
		EndReason:   relaycommon.StreamEndReasonDone,
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, -1, -1)
	require.Equal(t, float64(20_000), other["frt"],
		"request FRT must keep representing the user's total wait across retries")
	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, int64(1500), adminInfo["final_attempt_frt"],
		"admins need the final channel attempt FRT to avoid blaming it for earlier retries")
}

func TestGenerateTextOtherInfoOmitsFinalAttemptFRTWithoutAttributableResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	requestStart := time.Unix(1_700_000_000, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         requestStart,
		FirstResponseTime: requestStart.Add(20 * time.Second),
		IsStream:          true,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}
	info.BeginChannelAttempt()

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, -1, -1)
	adminInfo := other["admin_info"].(map[string]interface{})
	_, exists := adminInfo["final_attempt_frt"]
	require.False(t, exists, "missing attempt-local timing must not be logged as a misleading zero")
}

func TestGenerateTextOtherInfoOmitsFinalAttemptFRTForNonStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	requestStart := time.Unix(1_700_000_000, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         requestStart,
		FirstResponseTime: requestStart.Add(20 * time.Second),
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}
	attemptStart := info.BeginChannelAttempt()
	info.StreamStatus = &relaycommon.StreamStatus{
		StartedAt:   attemptStart,
		FirstDataAt: attemptStart.Add(1500 * time.Millisecond),
		EndReason:   relaycommon.StreamEndReasonDone,
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, -1, -1)
	adminInfo := other["admin_info"].(map[string]interface{})
	_, exists := adminInfo["final_attempt_frt"]
	require.False(t, exists, "non-streaming completion timing must not be labeled as first-token latency")
}
