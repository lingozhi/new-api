package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskPricePerSecondModelsIncludeDurationSettledMedia(t *testing.T) {
	assert.Contains(t, TaskPricePerSecondModels, "depth-anything-v2-small-video")
	assert.Contains(t, TaskPricePerSecondModels, "subtitle-remove")
	assert.NotContains(t, TaskPricePerSecondModels, "background-remove")
}
