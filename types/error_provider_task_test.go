package types

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderTaskPollingRetryErrorCarriesRetryAfterThroughNewAPIError(t *testing.T) {
	cause := errors.New("provider still generating")
	retryErr := NewProviderTaskPollingRetryError(cause, 17*time.Second)
	apiErr := NewError(retryErr, ErrorCodeBadResponse)

	retryAfter, ok := ProviderTaskPollingRetryAfter(apiErr)
	require.True(t, ok)
	assert.Equal(t, 17*time.Second, retryAfter)
	assert.ErrorIs(t, apiErr, ErrProviderTaskPollingRetryable)
	assert.ErrorIs(t, apiErr, cause)
}

func TestSetMaskedProviderMessagePreservesPollingMetadataWithoutRawCause(t *testing.T) {
	cause := errors.New("provider still generating with a sensitive credential")
	apiErr := NewErrorWithStatusCode(
		NewProviderTaskPollingRetryError(cause, 17*time.Second),
		ErrorCodeBadResponse,
		http.StatusAccepted,
	)

	apiErr.SetMaskedProviderMessage("provider still generating with ***")

	assert.Equal(t, "provider still generating with ***", apiErr.Error())
	assert.NotContains(t, apiErr.Error(), "sensitive credential")
	assert.ErrorIs(t, apiErr, ErrProviderTaskPollingRetryable)
	assert.NotErrorIs(t, apiErr, cause)
	retryAfter, ok := ProviderTaskPollingRetryAfter(apiErr)
	require.True(t, ok)
	assert.Equal(t, 17*time.Second, retryAfter)
	assert.Equal(t, "provider still generating with ***", apiErr.ToOpenAIError().Message)
	encoded, err := json.Marshal(apiErr)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "sensitive credential")
}

func TestSetMaskedProviderMessagePreservesUnsafeResubmissionWithoutRawCause(t *testing.T) {
	cause := errors.New("accepted provider failure with a sensitive credential")
	apiErr := NewError(NewProviderTaskUnsafeToResubmitError(cause), ErrorCodeBadResponse)

	apiErr.SetMaskedProviderMessage("accepted provider failure with ***")

	assert.Equal(t, "accepted provider failure with ***", apiErr.Error())
	assert.ErrorIs(t, apiErr, ErrProviderTaskUnsafeToResubmit)
	assert.NotErrorIs(t, apiErr, cause)
}

func TestSetMaskedProviderMessagePreservesPollingSentinelWithoutInventingDelay(t *testing.T) {
	cause := errors.New("transient provider poll failure with a sensitive credential")
	apiErr := NewError(errors.Join(ErrProviderTaskPollingRetryable, cause), ErrorCodeBadResponse)

	apiErr.SetMaskedProviderMessage("transient provider poll failure with ***")

	assert.Equal(t, "transient provider poll failure with ***", apiErr.Error())
	assert.ErrorIs(t, apiErr, ErrProviderTaskPollingRetryable)
	assert.NotErrorIs(t, apiErr, cause)
	_, hasRetryAfter := ProviderTaskPollingRetryAfter(apiErr)
	assert.False(t, hasRetryAfter)
}

func TestSetMaskedProviderMessageSanitizesTypedRelayErrors(t *testing.T) {
	const rawSecret = "sensitive-provider-credential"
	tests := []struct {
		name       string
		apiErr     *NewAPIError
		clientText func(*NewAPIError) string
	}{
		{
			name: "openai",
			apiErr: WithOpenAIError(OpenAIError{
				Message:  "provider failed with " + rawSecret,
				Type:     "upstream_error",
				Code:     "bad_response",
				Metadata: json.RawMessage(`{"credential":"` + rawSecret + `"}`),
			}, http.StatusBadGateway),
			clientText: func(apiErr *NewAPIError) string { return apiErr.ToOpenAIError().Message },
		},
		{
			name:       "claude",
			apiErr:     WithClaudeError(ClaudeError{Message: "provider failed with " + rawSecret, Type: "upstream_error"}, http.StatusBadGateway),
			clientText: func(apiErr *NewAPIError) string { return apiErr.ToClaudeError().Message },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.apiErr.SetMaskedProviderMessage("provider failed with ***")

			assert.Equal(t, "provider failed with ***", test.clientText(test.apiErr))
			encoded, err := json.Marshal(test.apiErr)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), rawSecret)
		})
	}
}

func TestSetMessageInitializesMissingError(t *testing.T) {
	apiErr := &NewAPIError{}

	apiErr.SetMessage("masked provider error")

	assert.EqualError(t, apiErr, "masked provider error")
}

func TestProviderTaskUnsafeToResubmitErrorSurvivesWrapping(t *testing.T) {
	cause := errors.New("accepted task failed after possible billing")
	apiErr := NewError(NewProviderTaskUnsafeToResubmitError(cause), ErrorCodeBadResponse)

	assert.ErrorIs(t, apiErr, ErrProviderTaskUnsafeToResubmit)
	assert.ErrorIs(t, apiErr, cause)
	assert.True(t, IsProviderTaskUnsafeToResubmit(apiErr))
}

func TestUnmarkedProviderErrorDoesNotClaimUnsafeResubmission(t *testing.T) {
	err := errors.New("definitive pre-accept rejection")

	assert.False(t, IsProviderTaskUnsafeToResubmit(err))
	_, ok := ProviderTaskPollingRetryAfter(err)
	assert.False(t, ok)
}
