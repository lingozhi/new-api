package controller

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRelayAPIErrorPreservesUpstream429Provenance(t *testing.T) {
	model.ClearChannelCooldownsForTest()
	t.Cleanup(model.ClearChannelCooldownsForTest)

	taskErr := &dto.TaskError{
		StatusCode: http.StatusTooManyRequests,
		Error:      errors.New("upstream task rate limited"),
	}
	apiErr := taskRelayAPIError(taskErr)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.UpstreamStatusCode)
	assert.True(t, service.IsUpstreamRateLimitError(apiErr))

	processChannelError(
		newTestContext(),
		*types.NewChannelError(9010, 1, "task-rate-limited", false, "key", false),
		apiErr,
	)
	reason, expires, cooling := model.GetChannelCooldown(9010)
	require.True(t, cooling)
	assert.Contains(t, reason, "upstream_rate_limit")
	remaining := time.Until(time.Unix(expires, 0))
	assert.Greater(t, remaining, 119*time.Minute)
	assert.Less(t, remaining, 121*time.Minute)

	affinity := newTestContext()
	affinity.Set("channel_affinity_skip_retry_on_failure", true)
	assert.True(t, shouldRetryTaskRelay(affinity, false, taskErr, 1), "a genuine upstream 429 must switch even when the failed task channel was affinity-bound")
	assert.False(t, shouldRetryTaskRelay(newTestContext(), true, taskErr, 1), "an origin-locked task cannot safely switch providers or repeat the same rate-limited channel")
}

func TestTaskRelayAPIErrorLeavesLocal429Unattributed(t *testing.T) {
	localErr := &dto.TaskError{
		StatusCode: http.StatusTooManyRequests,
		LocalError: true,
		Error:      errors.New("local task rate limit"),
	}

	assert.Nil(t, taskRelayAPIError(nil))
	assert.Nil(t, taskRelayAPIError(localErr))
	assert.True(t, shouldRetryTaskRelay(newTestContext(), false, localErr, 1), "preserve the existing task retry behavior for local 429")
	assert.False(t, shouldRetryTaskRelay(newTestContext(), false, localErr, 0))

	pinned := newTestContext()
	pinned.Set("specific_channel_id", 1)
	assert.False(t, shouldRetryTaskRelay(pinned, false, localErr, 1))

	affinity := newTestContext()
	affinity.Set("channel_affinity_skip_retry_on_failure", true)
	assert.False(t, shouldRetryTaskRelay(affinity, false, localErr, 1), "local 429 must preserve the existing affinity policy")
}

func TestAutoDLSubmissionRefundDecisionMatchesUpstreamHTTPContract(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		statusCode int
		local      bool
		wantRefund bool
	}{
		{name: "400 request rejected", code: "fail_to_fetch_task", statusCode: http.StatusBadRequest, wantRefund: true},
		{name: "401 authentication rejected", code: "fail_to_fetch_task", statusCode: http.StatusUnauthorized, wantRefund: true},
		{name: "402 payment rejected", code: "fail_to_fetch_task", statusCode: http.StatusPaymentRequired, wantRefund: true},
		{name: "403 authorization rejected", code: "fail_to_fetch_task", statusCode: http.StatusForbidden, wantRefund: true},
		{name: "404 endpoint rejected", code: "fail_to_fetch_task", statusCode: http.StatusNotFound, wantRefund: true},
		{name: "405 method rejected", code: "fail_to_fetch_task", statusCode: http.StatusMethodNotAllowed, wantRefund: true},
		{name: "413 body rejected", code: "fail_to_fetch_task", statusCode: http.StatusRequestEntityTooLarge, wantRefund: true},
		{name: "415 media type rejected", code: "fail_to_fetch_task", statusCode: http.StatusUnsupportedMediaType, wantRefund: true},
		{name: "422 content rejected", code: "fail_to_fetch_task", statusCode: http.StatusUnprocessableEntity, wantRefund: true},
		{name: "408 response timeout is ambiguous", code: "fail_to_fetch_task", statusCode: http.StatusRequestTimeout},
		{name: "409 conflict may refer to an existing task", code: "fail_to_fetch_task", statusCode: http.StatusConflict},
		{name: "418 unrecognised standard 4xx is ambiguous", code: "fail_to_fetch_task", statusCode: http.StatusTeapot},
		{name: "429 throttling response is ambiguous", code: "fail_to_fetch_task", statusCode: http.StatusTooManyRequests},
		{name: "499 custom proxy status is ambiguous", code: "fail_to_fetch_task", statusCode: 499},
		{name: "598 custom proxy status is ambiguous", code: "fail_to_fetch_task", statusCode: 598},
		{name: "local lookalike cannot prove rejection", code: "fail_to_fetch_task", statusCode: http.StatusBadRequest, local: true},
		{name: "response read failure is ambiguous", code: "read_upstream_error_failed", statusCode: http.StatusBadGateway},
		{name: "malformed success response is ambiguous", code: "invalid_upstream_response", statusCode: http.StatusBadGateway},
		{name: "nil error is ambiguous"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var taskErr *dto.TaskError
			if test.code != "" {
				if test.local {
					taskErr = service.TaskErrorWrapperLocal(errors.New("local response"), test.code, test.statusCode)
				} else {
					taskErr = service.TaskErrorWrapper(errors.New("upstream response"), test.code, test.statusCode)
				}
			}
			assert.Equal(t, test.wantRefund, autoDLSubmissionWasExplicitlyRejected(taskErr))
		})
	}
}
