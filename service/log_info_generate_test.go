package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
