package model

import "github.com/QuantumNous/new-api/common"

// SetInputLog keeps submitted user content encrypted at rest independently of task results.
func (task *Task) SetInputLog(input string) {
	if input == "" {
		return
	}
	encrypted, err := common.EncryptString(input)
	if err != nil {
		common.SysError("encrypt task input log failed")
		return
	}
	task.PrivateData.InputLog = encrypted
}

func (task *Task) InputLog() string {
	if task.PrivateData.InputLog == "" {
		return ""
	}
	input, err := common.DecryptString(task.PrivateData.InputLog)
	if err != nil {
		common.SysError("decrypt task input log failed")
		return ""
	}
	return input
}
