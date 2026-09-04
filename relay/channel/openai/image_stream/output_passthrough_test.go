package image_stream

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageOutputPassesThroughWithoutDecoding(t *testing.T) {
	// Byte validation/materialization happens elsewhere. This step must not
	// decode, parse dimensions, or impose an output encoding on provider data.
	images := []dto.ImageData{{B64Json: "opaque-provider-image"}}
	require.NoError(t, validateAsyncImageOutput(context.Background(), images, asyncImageOutputContract{count: 1}))
	assert.ErrorIs(t, validateAsyncImageOutput(context.Background(), images, asyncImageOutputContract{count: 2}), ErrImageCountMismatch)
	assert.ErrorIs(t, validateAsyncImageOutput(context.Background(), nil, asyncImageOutputContract{count: 1}), ErrUndecodableImage)
}
