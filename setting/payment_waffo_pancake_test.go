package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWaffoPancakeEnvironmentValidation(t *testing.T) {
	assert.Empty(t, WaffoPancakeEnvironment)
	assert.True(t, IsValidWaffoPancakeEnvironment(WaffoPancakeEnvironmentTest))
	assert.True(t, IsValidWaffoPancakeEnvironment(WaffoPancakeEnvironmentProd))
	assert.False(t, IsValidWaffoPancakeEnvironment(""))
	assert.False(t, IsValidWaffoPancakeEnvironment("production"))
}
