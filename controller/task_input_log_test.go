package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskInputLogOnlyReturnedByTaskLogView(t *testing.T) {
	old := common.CryptoSecret
	common.CryptoSecret = "input-log-test-key"
	t.Cleanup(func() { common.CryptoSecret = old })
	task := &model.Task{TaskID: "task_input", UserId: 12}
	task.SetInputLog(`{"prompt":"A calm forest"}`)
	logs := tasksToDto([]*model.Task{task}, false)
	require.Len(t, logs, 1)
	assert.JSONEq(t, `{"prompt":"A calm forest"}`, logs[0].InputLog)
	assert.Empty(t, relay.TaskModel2Dto(task).InputLog)
	assert.Empty(t, task.Properties.Input)
}
