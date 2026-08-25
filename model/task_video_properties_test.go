package model

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPropertiesPreserveMiniMaxVideoMetadata(t *testing.T) {
	original := Properties{
		OriginModelName: "MiniMax-H3",
		Video: &relaycommon.TaskVideoProperties{
			Resolution:      "768P",
			Duration:        12,
			Ratio:           "16:9",
			InputImageCount: 2,
		},
	}

	value, err := original.Value()
	require.NoError(t, err)
	encoded, ok := value.([]byte)
	require.True(t, ok)

	var restored Properties
	require.NoError(t, restored.Scan(encoded))
	require.NotNil(t, restored.Video)
	assert.Equal(t, original.OriginModelName, restored.OriginModelName)
	assert.Equal(t, *original.Video, *restored.Video)
}
