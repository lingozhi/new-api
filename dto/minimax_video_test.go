package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMiniMaxVideoRequestNormalizesCallbackURL(t *testing.T) {
	callbackURL := "  https://callbacks.example.com/minimax  "
	request := &MiniMaxVideoGenerationV2Request{CallbackURL: &callbackURL}

	assert.Equal(t, "https://callbacks.example.com/minimax", request.NormalizeCallbackURL())
	if assert.NotNil(t, request.CallbackURL) {
		assert.Equal(t, "https://callbacks.example.com/minimax", *request.CallbackURL)
	}

	blank := " \t "
	request.CallbackURL = &blank
	assert.Empty(t, request.NormalizeCallbackURL())
	assert.Nil(t, request.CallbackURL)
}
