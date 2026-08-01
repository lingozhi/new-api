package types

import (
	"errors"
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
