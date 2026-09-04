package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskInputLogEncryptionRoundTrip(t *testing.T) {
	old := common.CryptoSecret
	common.CryptoSecret = "task-input-log-test-key"
	t.Cleanup(func() { common.CryptoSecret = old })
	task := &Task{}
	task.SetInputLog(`{"prompt":"private user prompt"}`)
	stored, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), "private user prompt")
	var restored Task
	require.NoError(t, common.Unmarshal(stored, &restored.PrivateData))
	assert.Equal(t, `{"prompt":"private user prompt"}`, restored.InputLog())
}
