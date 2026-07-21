package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreCommitStreamCapacityMarkerIsExplicit(t *testing.T) {
	marked := NewOpenAIError(
		errors.New("capacity"),
		ErrorCode("server_error"),
		http.StatusServiceUnavailable,
		ErrOptionWithPreCommitStreamCapacity(),
	)
	unmarked := NewOpenAIError(errors.New("capacity"), ErrorCode("server_error"), http.StatusServiceUnavailable)

	assert.True(t, IsPreCommitStreamCapacityError(marked))
	assert.False(t, IsPreCommitStreamCapacityError(unmarked))
	assert.False(t, IsPreCommitStreamCapacityError(nil))
}
