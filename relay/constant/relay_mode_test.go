package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageEditAliasesShareRelayMode(t *testing.T) {
	assert.Equal(t, RelayModeImagesEdits, Path2RelayMode("/v1/images/edits"))
	assert.Equal(t, RelayModeImagesEdits, Path2RelayMode("/v1/edits"))
}

func TestImageJobAndStoredGenerationPathsShareRelayMode(t *testing.T) {
	assert.Equal(t, RelayModeImagesGenerations, Path2RelayMode("/v1/jobs"))
	assert.Equal(t, RelayModeImagesGenerations, Path2RelayMode("/v1/jobs/task_123"))
	assert.Equal(t, RelayModeImagesGenerations, Path2RelayMode("/v1/images/generations"))
}

func TestMiniMaxVideoGenerationV2PathsUseVideoRelayModes(t *testing.T) {
	assert.Equal(t, RelayModeVideoSubmit, Path2RelayMode("/v2/video_generation"))
	assert.Equal(t, RelayModeVideoFetchByID, Path2RelayMode("/v2/query/video_generation/task_123"))
}
