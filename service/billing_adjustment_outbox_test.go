package service

import (
	"bytes"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func captureBillingAdjustmentDrainLog(t *testing.T, drainErr error) (string, string) {
	t.Helper()

	var infoLog bytes.Buffer
	var warningLog bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter = &infoLog
	gin.DefaultErrorWriter = &warningLog
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		gin.DefaultErrorWriter = oldErrorWriter
		common.LogWriterMu.Unlock()
	})

	logBillingAdjustmentDrainResult(0, 1, drainErr)
	return infoLog.String(), warningLog.String()
}

func TestBillingAdjustmentBalanceHeadroomWaitLogsAtInfo(t *testing.T) {
	infoLog, warningLog := captureBillingAdjustmentDrainLog(t, errors.Join(
		model.ErrBillingAdjustmentBalanceBlocked,
		errors.New("wallet current=3327038316 delta=17959"),
	))

	assert.Contains(t, infoLog, "[INFO]")
	assert.Contains(t, infoLog, "waiting for balance headroom")
	assert.Empty(t, warningLog)
}

func TestBillingAdjustmentUnexpectedDrainFailureRemainsWarning(t *testing.T) {
	infoLog, warningLog := captureBillingAdjustmentDrainLog(t, errors.New("database unavailable"))

	assert.Empty(t, infoLog)
	assert.Contains(t, warningLog, "[WARN]")
	assert.Contains(t, warningLog, "billing adjustment outbox drain incomplete")
}
