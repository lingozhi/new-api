package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

// TestChannelHealthOutcomeStatusScoresEmptyUpstreamAsFailure covers the pure
// status-mapping helper: a real upstream error keeps its own status, a clean
// success is 200, and a 200-but-empty upstream response (UpstreamEmptyResponse)
// is scored 502 so the circuit treats a channel that silently returns nothing
// as failing rather than healthy.
func TestChannelHealthOutcomeStatusScoresEmptyUpstreamAsFailure(t *testing.T) {
	upstreamErr := types.NewErrorWithStatusCode(errors.New("do request failed"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	mappedUpstreamErr := types.NewErrorWithStatusCode(errors.New("upstream overloaded"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
	mappedUpstreamErr.UpstreamStatusCode = http.StatusServiceUnavailable
	streamCapacityErr := types.NewOpenAIError(
		errors.New("We're currently experiencing high demand, which may cause temporary errors."),
		types.ErrorCode("server_error"),
		http.StatusBadRequest,
		types.ErrOptionWithPreCommitStreamCapacity(),
	)
	localErr := types.NewErrorWithStatusCode(errors.New("convert request failed"), types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError)
	failedStream := relaycommon.NewStreamStatus()
	failedStream.SetEndReason(relaycommon.StreamEndReasonUpstreamFailed, errors.New("upstream response.failed"))
	clientTerminalStream := relaycommon.NewStreamStatus()
	clientTerminalStream.SetEndReason(relaycommon.StreamEndReasonTerminalClientError, errors.New("invalid prompt"))
	internalFailureStream := relaycommon.NewStreamStatus()
	internalFailureStream.SetEndReason(relaycommon.StreamEndReasonInternalError, errors.New("conversion failed"))

	tests := []struct {
		name         string
		apiErr       *types.NewAPIError
		relayInfo    *relaycommon.RelayInfo
		wantStatus   int
		wantLocalErr bool
	}{
		{name: "clean success", apiErr: nil, relayInfo: &relaycommon.RelayInfo{}, wantStatus: http.StatusOK, wantLocalErr: false},
		{name: "nil relay info", apiErr: nil, relayInfo: nil, wantStatus: http.StatusOK, wantLocalErr: false},
		{name: "empty upstream response", apiErr: nil, relayInfo: &relaycommon.RelayInfo{UpstreamEmptyResponse: true}, wantStatus: http.StatusBadGateway, wantLocalErr: false},
		{name: "upstream terminal failure", apiErr: nil, relayInfo: &relaycommon.RelayInfo{StreamStatus: failedStream}, wantStatus: http.StatusBadGateway, wantLocalErr: false},
		{name: "client terminal failure", apiErr: nil, relayInfo: &relaycommon.RelayInfo{StreamStatus: clientTerminalStream}, wantStatus: http.StatusBadRequest, wantLocalErr: false},
		{name: "internal stream failure", apiErr: nil, relayInfo: &relaycommon.RelayInfo{StreamStatus: internalFailureStream}, wantStatus: http.StatusInternalServerError, wantLocalErr: true},
		{name: "client terminal failure wins over empty usage", apiErr: nil, relayInfo: &relaycommon.RelayInfo{StreamStatus: clientTerminalStream, UpstreamEmptyResponse: true}, wantStatus: http.StatusBadRequest, wantLocalErr: false},
		{name: "upstream error wins over empty flag", apiErr: upstreamErr, relayInfo: &relaycommon.RelayInfo{UpstreamEmptyResponse: true}, wantStatus: http.StatusBadGateway, wantLocalErr: false},
		{name: "immutable upstream status wins over client mapping", apiErr: mappedUpstreamErr, relayInfo: &relaycommon.RelayInfo{}, wantStatus: http.StatusServiceUnavailable, wantLocalErr: false},
		{name: "marked stream capacity survives client mapping", apiErr: streamCapacityErr, relayInfo: &relaycommon.RelayInfo{}, wantStatus: http.StatusServiceUnavailable, wantLocalErr: false},
		{name: "gateway-local error", apiErr: localErr, relayInfo: &relaycommon.RelayInfo{}, wantStatus: http.StatusInternalServerError, wantLocalErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, localErr := channelHealthOutcomeStatus(tt.apiErr, tt.relayInfo)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if localErr != tt.wantLocalErr {
				t.Fatalf("localError = %v, want %v", localErr, tt.wantLocalErr)
			}
		})
	}
}

func TestChannelHealthPathNormalizesPublicAsyncImageJobsRoute(t *testing.T) {
	assert.Equal(t, "/v1/jobs", ChannelHealthPath("/v1/jobs"))
	assert.Equal(t, "/v1/jobs", ChannelHealthPath("/v1/jobs/task_123?include=result"))
}

// TestRecordChannelHealthOutcomeCountsEmptyUpstreamResponse verifies the
// end-to-end effect: repeated 200-but-empty upstream responses (no apiErr, only
// the UpstreamEmptyResponse flag) open the channel circuit, so a channel that
// keeps truncating streams is routed away from instead of staying "healthy".
func TestRecordChannelHealthOutcomeCountsEmptyUpstreamResponse(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001426
	const modelName = "test-empty-upstream-response"
	const requestPath = "/v1/responses"

	emptyInfo := &relaycommon.RelayInfo{UpstreamEmptyResponse: true}

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, emptyInfo, time.Now(), nil, false)
	}

	if IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("expected repeated empty upstream responses to open the channel circuit")
	}
}

// TestRecordChannelHealthOutcomeHealthySuccessStaysAvailable is the contrast:
// a normal 200 with real output (no empty flag) must not open the circuit.
func TestRecordChannelHealthOutcomeHealthySuccessStaysAvailable(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001427
	const modelName = "test-healthy-success"
	const requestPath = "/v1/responses"

	healthyInfo := &relaycommon.RelayInfo{}

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, healthyInfo, time.Now(), nil, false)
	}

	if !IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("expected healthy successes to keep the channel available")
	}
}

// A RelayInfo is reused across channel retries. If an earlier attempt emitted
// data before failing, FirstResponseTime belongs to that earlier attempt. The
// current stream's FirstDataAt is the only attempt-local TTFT signal; falling
// back to time.Since(attemptStart) would score the whole successful stream as
// TTFT and can evict a healthy fallback channel.
func TestRecordChannelHealthOutcomeUsesCurrentAttemptFirstDataAfterRetry(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001433
	const modelName = "test-retry-attempt-first-data"
	const requestPath = "/v1/responses"

	healthKey := channelHealthKey(channelID, modelName, requestPath)
	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-20 * time.Second)
		status := &relaycommon.StreamStatus{
			StartedAt:   attemptStart.Add(500 * time.Millisecond),
			FirstDataAt: attemptStart.Add(time.Second),
			LastDataAt:  attemptStart.Add(19 * time.Second),
			EndedAt:     attemptStart.Add(19 * time.Second),
			EndReason:   relaycommon.StreamEndReasonDone,
		}
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart.Add(-30 * time.Second),
			FirstResponseTime: attemptStart.Add(-10 * time.Second),
			IsStream:          true,
			StreamStatus:      status,
		}

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
		assert.False(t, model.ShouldDemoteChannelPriority(healthKey),
			"a 1s fallback attempt must never inherit enough prior-request latency to be demoted")
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"a fallback channel with 1s current-attempt TTFT must not be scored with its 20s total stream duration")
	assert.InDelta(t, 0.5, model.GetChannelHealthScore(healthKey), 0.01,
		"the health score must reflect the current attempt's 1s TTFT")
}

func TestRecordChannelHealthOutcomeDoesNotScoreNonStreamingCompletionLatency(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001438
	const modelName = "test-non-stream-completion-latency"
	const requestPath = "/v1/chat/completions"
	healthKey := channelHealthKey(channelID, modelName, requestPath)

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-20 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart,
			FirstResponseTime: attemptStart.Add(15 * time.Second),
			RelayMode:         relayconstant.RelayModeChatCompletions,
		}

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"non-streaming completion time is not TTFT and must not open the slow circuit")
	assert.False(t, model.ShouldDemoteChannelPriority(healthKey),
		"non-streaming completion time must not lower the shared channel priority")
	assert.Equal(t, 1.0, model.GetChannelHealthScore(healthKey),
		"non-streaming successes without TTFT must leave the latency score neutral")
}

func TestRecordChannelHealthOutcomeDoesNotUseStreamDurationWhenCurrentAttemptHasNoData(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001434
	const modelName = "test-retry-attempt-no-current-data"
	const requestPath = "/v1/responses"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-20 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart.Add(-30 * time.Second),
			FirstResponseTime: attemptStart.Add(-10 * time.Second),
			IsStream:          true,
			StreamStatus: &relaycommon.StreamStatus{
				StartedAt: attemptStart.Add(500 * time.Millisecond),
				EndedAt:   attemptStart.Add(19 * time.Second),
				EndReason: relaycommon.StreamEndReasonDone,
			},
		}

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"a current stream without attributable first data must not use total stream duration as TTFT")
}

func TestRecordChannelHealthOutcomeDoesNotUseStreamDurationWhenStreamStatusIsUnavailable(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001436
	const modelName = "test-retry-stream-without-status"
	const requestPath = "/v1/chat/completions"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-20 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart.Add(-30 * time.Second),
			FirstResponseTime: attemptStart.Add(-10 * time.Second),
			IsStream:          true,
		}

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"a streaming adapter without StreamStatus must not use total stream duration as TTFT")
}

func TestRecordChannelHealthOutcomeCountsGenuinelySlowCurrentAttemptAfterRetry(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001435
	const modelName = "test-retry-attempt-slow-current-data"
	const requestPath = "/v1/responses"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-12 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart.Add(-30 * time.Second),
			FirstResponseTime: attemptStart.Add(-10 * time.Second),
			IsStream:          true,
			StreamStatus: &relaycommon.StreamStatus{
				StartedAt:   attemptStart.Add(500 * time.Millisecond),
				FirstDataAt: attemptStart.Add(10 * time.Second),
				LastDataAt:  attemptStart.Add(11 * time.Second),
				EndedAt:     attemptStart.Add(11 * time.Second),
				EndReason:   relaycommon.StreamEndReasonDone,
			},
		}

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"a fallback channel with genuinely slow current-attempt TTFT must still open the circuit")
}

func TestRecordChannelHealthOutcomeCountsSlowCurrentAttemptWithoutStreamStatus(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001437
	const modelName = "test-retry-slow-stream-without-status"
	const requestPath = "/v1/chat/completions"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-10 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart.Add(-30 * time.Second),
			FirstResponseTime: attemptStart.Add(-10 * time.Second),
			IsStream:          true,
			StreamStatus: &relaycommon.StreamStatus{
				StartedAt:   attemptStart.Add(-20 * time.Second),
				FirstDataAt: attemptStart.Add(-19 * time.Second),
				EndReason:   relaycommon.StreamEndReasonUpstreamFailed,
			},
		}
		info.BeginChannelAttempt()
		info.SetFirstResponseTime()

		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"a genuinely slow current attempt must be scored even when its adapter has no StreamStatus")
}

// TestClientCanceledIsNotAttributedToChannel guards the mis-attribution bug seen
// in prod: a client aborting mid-request makes the in-flight upstream call fail
// with context.Canceled, surfaced as ErrorCodeDoRequestFailed. Since the retry
// loop touches several channels, one cancellation was recording a failure — and
// a 5m cooldown — against every channel it tried, all of them healthy.
func TestClientCanceledIsNotAttributedToChannel(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001428
	const modelName = "test-client-canceled"
	const requestPath = "/v1/messages"

	// Exactly how doRequest surfaces a client abort: context.Canceled wrapped as
	// a do-request-failed channel error.
	canceledErr := types.NewErrorWithStatusCode(
		fmt.Errorf("do request failed: %w", context.Canceled),
		types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now(), canceledErr, false)
	}

	if !IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("a client-canceled request must not open a healthy channel's circuit")
	}
}

// TestDeadlineExceededStillCountsAgainstChannel is the contrast that keeps the
// fix narrow: our own timeout firing (context.DeadlineExceeded) does mean the
// channel is slow or dead, so it must still be attributed.
func TestDeadlineExceededStillCountsAgainstChannel(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001429
	const modelName = "test-deadline-exceeded"
	const requestPath = "/v1/messages"

	timeoutErr := types.NewErrorWithStatusCode(
		fmt.Errorf("do request failed: %w", context.DeadlineExceeded),
		types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now(), timeoutErr, false)
	}

	if IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("repeated upstream timeouts must still open the channel circuit")
	}
}

func TestChannelHealthPathNormalizesBoundedRouteFamilies(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/v1/responses?beta=true", want: "/v1/responses"},
		{path: "/pg/chat/completions", want: "/v1/chat/completions"},
		{path: "/v1beta/models/gemini-2.5-pro:generateContent", want: "/gemini/generate"},
		{path: "/v1beta/models/gemini-2.5-pro:streamGenerateContent", want: "/gemini/stream_generate"},
		{path: "/v1/videos/task-123", want: "/v1/tasks"},
		{path: "/v1/edits", want: "/v1/images/edits"},
		{path: "/proxy/v1/edits?legacy=true", want: "/v1/images/edits"},
		{path: "/arbitrary/user-controlled/value", want: "/other"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := ChannelHealthPath(tt.path); got != tt.want {
				t.Fatalf("ChannelHealthPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTextGenerationHealthRequest(t *testing.T) {
	tests := []struct {
		name        string
		relayMode   int
		requestPath string
		want        bool
	}{
		{name: "chat completions", relayMode: relayconstant.RelayModeChatCompletions, requestPath: "/v1/chat/completions", want: true},
		{name: "legacy completions", relayMode: relayconstant.RelayModeCompletions, requestPath: "/v1/completions", want: true},
		{name: "responses", relayMode: relayconstant.RelayModeResponses, requestPath: "/v1/responses", want: true},
		{name: "claude messages path fallback", requestPath: "/v1/messages", want: true},
		{name: "gemini generation", relayMode: relayconstant.RelayModeGemini, requestPath: "/v1beta/models/gemini-2.5-pro:generateContent", want: true},
		{name: "gemini streaming generation", relayMode: relayconstant.RelayModeGemini, requestPath: "/v1beta/models/gemini-2.5-pro:streamGenerateContent", want: true},
		{name: "gemini embedding", relayMode: relayconstant.RelayModeGemini, requestPath: "/v1beta/models/text-embedding-004:embedContent", want: false},
		{name: "responses compact", relayMode: relayconstant.RelayModeResponsesCompact, requestPath: "/v1/responses/compact", want: false},
		{name: "embedding", relayMode: relayconstant.RelayModeEmbeddings, requestPath: "/v1/embeddings", want: false},
		{name: "rerank", relayMode: relayconstant.RelayModeRerank, requestPath: "/v1/rerank", want: false},
		{name: "audio", relayMode: relayconstant.RelayModeAudioSpeech, requestPath: "/v1/audio/speech", want: false},
		{name: "task", relayMode: relayconstant.RelayModeVideoSubmit, requestPath: "/v1/videos", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{RelayMode: tt.relayMode}
			assert.Equal(t, tt.want, IsTextGenerationHealthRequest(info, tt.requestPath))
		})
	}
}

func TestRecordChannelHealthOutcomeKeepsNonStreamingOperationalLatency(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001439
	const modelName = "test-non-stream-embedding-latency"
	const requestPath = "/v1/embeddings"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-10 * time.Second)
		info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings}
		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"non-streaming operational routes must retain completion-latency health scoring")
}

func TestRecordChannelHealthOutcomeDoesNotScoreNonStreamingGeminiCompletionLatency(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001441
	const modelName = "test-non-stream-gemini-latency"
	const requestPath = "/v1beta/models/gemini-2.5-pro:generateContent"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-10 * time.Second)
		info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeGemini}
		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"Gemini generateContent may share its health key with alt=sse traffic, so completion time must not be scored as TTFT")
}

func TestRecordChannelHealthOutcomeDoesNotUseGeminiStreamDurationWithoutFirstData(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001442
	const modelName = "test-gemini-stream-without-first-data"
	const requestPath = "/v1beta/models/gemini-2.5-pro:streamGenerateContent"

	for i := 0; i < 3; i++ {
		attemptStart := time.Now().Add(-20 * time.Second)
		info := &relaycommon.RelayInfo{
			IsStream:  true,
			RelayMode: relayconstant.RelayModeGemini,
			StreamStatus: &relaycommon.StreamStatus{
				StartedAt: attemptStart,
				EndedAt:   attemptStart.Add(19 * time.Second),
				EndReason: relaycommon.StreamEndReasonDone,
			},
		}
		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	assert.True(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"Gemini stream duration without attributable first data must not be treated as TTFT")
}

func TestRecordChannelHealthOutcomeUnknownTextLatencyPreservesStreamingSlowness(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001440
	const modelName = "test-mixed-stream-latency"
	const requestPath = "/v1/responses"
	healthKey := channelHealthKey(channelID, modelName, requestPath)

	recordSlowStream := func() {
		attemptStart := time.Now().Add(-10 * time.Second)
		info := &relaycommon.RelayInfo{
			IsStream: true,
			StreamStatus: &relaycommon.StreamStatus{
				StartedAt:   attemptStart,
				FirstDataAt: attemptStart.Add(10 * time.Second),
				LastDataAt:  attemptStart.Add(10 * time.Second),
				EndedAt:     attemptStart.Add(10 * time.Second),
				EndReason:   relaycommon.StreamEndReasonDone,
			},
		}
		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}
	recordUnknownNonStream := func() {
		attemptStart := time.Now().Add(-20 * time.Second)
		info := &relaycommon.RelayInfo{
			StartTime:         attemptStart,
			FirstResponseTime: attemptStart.Add(15 * time.Second),
			RelayMode:         relayconstant.RelayModeResponses,
		}
		RecordChannelHealthOutcome(channelID, modelName, requestPath, info, attemptStart, nil, false)
	}

	recordSlowStream()
	recordUnknownNonStream()
	recordSlowStream()
	assert.True(t, model.ShouldDemoteChannelPriority(healthKey),
		"unknown non-streaming TTFT must not erase measured slow-stream evidence")

	recordUnknownNonStream()
	recordSlowStream()
	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"three measured slow streams must still open the circuit when non-stream traffic is interleaved")
}

func TestRecordChannelHealthOutcomeDoesNotScoreImageCompletionLatency(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	tests := []struct {
		name        string
		channelID   int
		modelName   string
		requestPath string
		relayInfo   func(time.Time) *relaycommon.RelayInfo
	}{
		{
			name:        "responses image generation",
			channelID:   9001430,
			modelName:   "test-async-image-response-latency",
			requestPath: "/v1/images/generations",
		},
		{
			name:        "generic image edit alias",
			channelID:   9001431,
			modelName:   "test-async-image-edit-latency",
			requestPath: "/v1/edits",
			relayInfo: func(start time.Time) *relaycommon.RelayInfo {
				return &relaycommon.RelayInfo{
					StartTime:         start,
					FirstResponseTime: start.Add(20 * time.Second),
				}
			},
		},
		{
			name:        "unified async image job",
			channelID:   9001444,
			modelName:   "test-async-image-jobs-latency",
			requestPath: "/v1/jobs/task_123",
			relayInfo: func(start time.Time) *relaycommon.RelayInfo {
				return &relaycommon.RelayInfo{
					StartTime:         start,
					FirstResponseTime: start.Add(15 * time.Minute),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 5; i++ {
				attemptStart := time.Now().Add(-30 * time.Second)
				var info *relaycommon.RelayInfo
				if tt.relayInfo != nil {
					info = tt.relayInfo(attemptStart)
				}
				RecordChannelHealthOutcome(tt.channelID, tt.modelName, tt.requestPath, info, attemptStart, nil, false)
			}
			assert.True(t, IsChannelHealthAvailable(tt.channelID, tt.modelName, tt.requestPath))
		})
	}
}

func TestRecordChannelHealthOutcomeStillCountsImageFailures(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001432
	const modelName = "test-async-image-upstream-failure"
	const requestPath = "/v1/images/generations"
	upstreamErr := types.NewErrorWithStatusCode(errors.New("image upstream failed"), types.ErrorCodeBadResponse, http.StatusBadGateway)

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now().Add(-30*time.Second), upstreamErr, false)
	}

	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath))
}

func TestRecordChannelHealthOutcomeCountsRelayServiceTransient400(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001443
	const modelName = "test-relay-service-transient-400"
	const requestPath = "/v1/responses"
	err := types.NewErrorWithStatusCode(
		errors.New("Upstream request failed, please try again, 请重试 (Relay Service)"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	err.UpstreamStatusCode = http.StatusBadRequest

	for i := 0; i < 3; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now(), err, false)
	}

	assert.False(t, IsChannelHealthAvailable(channelID, modelName, requestPath),
		"the confirmed upstream relay outage must count as a channel failure despite its HTTP 400 wrapper")
}

// TestRecordChannelHealthOutcomeIgnoresGatewayLocalErrors verifies that
// failures which never reached the upstream channel (request conversion,
// pricing, serialization — the gateway's own processing) don't open a
// healthy channel's circuit. Only errors attributable to the upstream
// channel itself should count against adaptive health.
func TestRecordChannelHealthOutcomeIgnoresGatewayLocalErrors(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001424
	const modelName = "test-local-error-classification"
	const requestPath = "/v1/responses"

	localErr := types.NewErrorWithStatusCode(errors.New("failed to copy request to GeneralOpenAIRequest"), types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError)

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now(), localErr, false)
	}

	if !IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("expected gateway-local errors (e.g. request conversion failures) not to open the channel circuit")
	}
}

// TestRecordChannelHealthOutcomeCountsChannelAttributableErrors is the
// contrasting case: a genuine upstream failure (do-request failed) must
// still open the circuit after the failure threshold.
func TestRecordChannelHealthOutcomeCountsChannelAttributableErrors(t *testing.T) {
	oldEnabled := common.AdaptiveChannelHealthEnabled
	common.AdaptiveChannelHealthEnabled = true
	t.Cleanup(func() { common.AdaptiveChannelHealthEnabled = oldEnabled })

	const channelID = 9001425
	const modelName = "test-upstream-error-classification"
	const requestPath = "/v1/responses"

	upstreamErr := types.NewErrorWithStatusCode(errors.New("do request failed"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	for i := 0; i < 5; i++ {
		RecordChannelHealthOutcome(channelID, modelName, requestPath, nil, time.Now(), upstreamErr, false)
	}

	if IsChannelHealthAvailable(channelID, modelName, requestPath) {
		t.Fatal("expected repeated do-request-failed errors to open the channel circuit")
	}
}
