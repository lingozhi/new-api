package model

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	DB = db
	LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	initCol()

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&Task{},
		&TaskWebhook{},
		&ImageBillingReservation{},
		&ImageTaskArtifactChunk{},
		&ImageInputCleanup{},
		&ImageTaskBillingLogOutbox{},
		&ImageTaskBillingLogReceipt{},
		&BillingAdjustmentOutbox{},
		&Midjourney{},
		&User{},
		&Token{},
		&PasskeyCredential{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Log{},
		&Channel{},
		&QuotaData{},
		&Ability{},
		&TopUp{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&UserOAuthBinding{},
		&PerfMetric{},
		&SystemInstance{},
		&SystemTask{},
		&SystemTaskLock{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

func truncateTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM task_webhooks")
		DB.Exec("DELETE FROM image_billing_reservations")
		DB.Exec("DELETE FROM image_task_artifact_chunks")
		DB.Exec("DELETE FROM image_input_cleanups")
		DB.Exec("DELETE FROM image_task_billing_log_outboxes")
		DB.Exec("DELETE FROM image_task_billing_log_receipts")
		DB.Exec("DELETE FROM billing_adjustment_outboxes")
		DB.Exec("DELETE FROM midjourneys")
		DB.Exec("DELETE FROM tasks")
		DB.Exec("DELETE FROM passkey_credentials")
		DB.Exec("DELETE FROM two_fa_backup_codes")
		DB.Exec("DELETE FROM two_fas")
		DB.Exec("DELETE FROM tokens")
		DB.Exec("DELETE FROM user_oauth_bindings")
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM logs")
		DB.Exec("DELETE FROM channels")
		DB.Exec("DELETE FROM quota_data")
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM top_ups")
		DB.Exec("DELETE FROM subscription_orders")
		DB.Exec("DELETE FROM subscription_plans")
		DB.Exec("DELETE FROM user_subscriptions")
		DB.Exec("DELETE FROM subscription_pre_consume_records")
		DB.Exec("DELETE FROM perf_metrics")
		DB.Exec("DELETE FROM system_instances")
		DB.Exec("DELETE FROM system_task_locks")
		DB.Exec("DELETE FROM system_tasks")
	})
}

func insertTask(t *testing.T, task *Task) {
	t.Helper()
	task.CreatedAt = time.Now().Unix()
	task.UpdatedAt = time.Now().Unix()
	require.NoError(t, DB.Create(task).Error)
}

// ---------------------------------------------------------------------------
// Snapshot / Equal — pure logic tests (no DB)
// ---------------------------------------------------------------------------

func TestSnapshotEqual_Same(t *testing.T) {
	s := taskSnapshot{
		Status:     TaskStatusInProgress,
		Progress:   "50%",
		StartTime:  1000,
		FinishTime: 0,
		FailReason: "",
		ResultURL:  "",
		Data:       json.RawMessage(`{"key":"value"}`),
	}
	assert.True(t, s.Equal(s))
}

func TestSnapshotEqual_DifferentStatus(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{}`)}
	b := taskSnapshot{Status: TaskStatusSuccess, Data: json.RawMessage(`{}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_DifferentProgress(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Progress: "30%", Data: json.RawMessage(`{}`)}
	b := taskSnapshot{Status: TaskStatusInProgress, Progress: "60%", Data: json.RawMessage(`{}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_DifferentData(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{"a":1}`)}
	b := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{"a":2}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_NilVsEmpty(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: nil}
	b := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage{}}
	// bytes.Equal(nil, []byte{}) == true
	assert.True(t, a.Equal(b))
}

func TestSnapshot_Roundtrip(t *testing.T) {
	task := &Task{
		Status:     TaskStatusInProgress,
		Progress:   "42%",
		StartTime:  1234,
		FinishTime: 5678,
		FailReason: "timeout",
		PrivateData: TaskPrivateData{
			ResultURL: "https://example.com/result.mp4",
		},
		Data: json.RawMessage(`{"model":"test-model"}`),
	}
	snap := task.Snapshot()
	assert.Equal(t, task.Status, snap.Status)
	assert.Equal(t, task.Progress, snap.Progress)
	assert.Equal(t, task.StartTime, snap.StartTime)
	assert.Equal(t, task.FinishTime, snap.FinishTime)
	assert.Equal(t, task.FailReason, snap.FailReason)
	assert.Equal(t, task.PrivateData.ResultURL, snap.ResultURL)
	assert.JSONEq(t, string(task.Data), string(snap.Data))
}

// ---------------------------------------------------------------------------
// UpdateWithStatus CAS — DB integration tests
// ---------------------------------------------------------------------------

func TestUpdateWithStatus_Win(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:   "task_cas_win",
		Status:   TaskStatusInProgress,
		Progress: "50%",
		Data:     json.RawMessage(`{}`),
	}
	insertTask(t, task)

	task.Status = TaskStatusSuccess
	task.Progress = "100%"
	won, err := task.UpdateWithStatus(TaskStatusInProgress)
	require.NoError(t, err)
	assert.True(t, won)

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, TaskStatusSuccess, reloaded.Status)
	assert.Equal(t, "100%", reloaded.Progress)
}

func TestUpdateWithStatus_Lose(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID: "task_cas_lose",
		Status: TaskStatusFailure,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	task.Status = TaskStatusSuccess
	won, err := task.UpdateWithStatus(TaskStatusInProgress) // wrong fromStatus
	require.NoError(t, err)
	assert.False(t, won)

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, TaskStatusFailure, reloaded.Status) // unchanged
}

func TestRefreshSuccessResultMergesLatestPrivateData(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:   "task_refresh_result",
		Status:   TaskStatusSuccess,
		Progress: "100%",
		PrivateData: TaskPrivateData{
			UpstreamTaskID: "upstream-refresh",
			ResultURL:      "https://cdn.example.com/old.mp4",
			TokenId:        10,
		},
		Data: json.RawMessage(`{"data":{"status":"SUCCESS"}}`),
	}
	insertTask(t, task)

	stale := *task
	latestPrivateData := task.PrivateData
	latestPrivateData.TokenId = 99
	latestPrivateData.BillingSource = "subscription"
	terminalUpdatedAt := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, DB.Model(&Task{}).Where("id = ?", task.ID).UpdateColumns(map[string]any{
		"private_data": latestPrivateData,
		"updated_at":   terminalUpdatedAt,
	}).Error)

	updated, err := stale.RefreshSuccessResult(
		"https://cdn.example.com/fresh.mp4",
		json.RawMessage(`{"code":"Success","data":{"status":"SUCCESS"}}`),
		1700000100,
	)
	require.NoError(t, err)
	assert.True(t, updated)

	var stored Task
	require.NoError(t, DB.First(&stored, task.ID).Error)
	assert.Equal(t, "https://cdn.example.com/fresh.mp4", stored.PrivateData.ResultURL)
	assert.Equal(t, 99, stored.PrivateData.TokenId)
	assert.Equal(t, "subscription", stored.PrivateData.BillingSource)
	assert.EqualValues(t, 1700000100, stored.PrivateData.ResultRefreshedAt)
	assert.JSONEq(t, `{"code":"Success","data":{"status":"SUCCESS"}}`, string(stored.Data))
	assert.Equal(t, terminalUpdatedAt, stored.UpdatedAt)

	unchanged, err := stored.RefreshSuccessResult(stored.PrivateData.ResultURL, stored.Data, stored.PrivateData.ResultRefreshedAt)
	require.NoError(t, err)
	assert.False(t, unchanged)

	var queriedAgain Task
	require.NoError(t, DB.First(&queriedAgain, task.ID).Error)
	assert.Equal(t, terminalUpdatedAt, queriedAgain.UpdatedAt)
}

func TestClaimSuccessResultRefreshFencesIndependentGatewayInstances(t *testing.T) {
	truncateTables(t)

	now := time.Now().Unix()
	task := &Task{
		TaskID:    "task_result_refresh_claim",
		Platform:  constant.TaskPlatformAutoDL,
		Status:    TaskStatusSuccess,
		UpdatedAt: now - 60,
		PrivateData: TaskPrivateData{
			ResultURL:         "https://cdn.example.com/expired.mp4",
			ResultRefreshedAt: now - 60,
		},
	}
	require.NoError(t, DB.Create(task).Error)
	terminalUpdatedAt := task.UpdatedAt

	firstInstance := *task
	secondInstance := *task
	claimed, err := firstInstance.ClaimSuccessResultRefresh(now-30, now)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = secondInstance.ClaimSuccessResultRefresh(now-30, now)
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.EqualValues(t, now, secondInstance.PrivateData.ResultRefreshedAt)

	var stored Task
	require.NoError(t, DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, now, stored.PrivateData.ResultRefreshedAt)
	assert.Equal(t, terminalUpdatedAt, stored.UpdatedAt)
}

func TestCompleteSubmissionCheckpointMakesTaskPollableAtomically(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:    "task_submission_checkpoint",
		Platform:  constant.TaskPlatformAutoDL,
		ChannelId: 60,
		Status:    TaskStatusCheckpointPending,
		Progress:  "0%",
		Quota:     321,
		PrivateData: TaskPrivateData{
			ChannelKeyHash: "stable-key-fingerprint",
		},
	}
	insertTask(t, task)
	assert.Empty(t, GetAllUnFinishSyncTasks(10))

	completed, err := task.CompleteSubmissionCheckpoint(
		"upstream-task-id",
		json.RawMessage(`{"code":"Success","data":{"status":"QUEUED"}}`),
		654,
	)
	require.NoError(t, err)
	assert.True(t, completed)

	var stored Task
	require.NoError(t, DB.First(&stored, task.ID).Error)
	assert.EqualValues(t, TaskStatusNotStart, stored.Status)
	assert.Equal(t, "upstream-task-id", stored.PrivateData.UpstreamTaskID)
	assert.Equal(t, "stable-key-fingerprint", stored.PrivateData.ChannelKeyHash)
	assert.Equal(t, 654, stored.Quota)
	assert.JSONEq(t, `{"code":"Success","data":{"status":"QUEUED"}}`, string(stored.Data))
	require.Len(t, GetAllUnFinishSyncTasks(10), 1)

	completed, err = task.CompleteSubmissionCheckpoint(
		"upstream-task-id",
		json.RawMessage(`{"code":"Success","data":{"status":"QUEUED"}}`),
		654,
	)
	require.NoError(t, err)
	assert.True(t, completed)
}

func TestFreshlyActivatedSubmissionCheckpointUsesActivationTimeForRecovery(t *testing.T) {
	truncateTables(t)

	now := time.Now().Unix()
	task := &Task{
		TaskID:     "task_fresh_activation_old_submission",
		Platform:   constant.TaskPlatformAutoDL,
		Status:     TaskStatusCheckpointPending,
		Progress:   "0%",
		SubmitTime: now - int64((20*time.Minute)/time.Second),
		CreatedAt:  now - int64((20*time.Minute)/time.Second),
		UpdatedAt:  now,
	}
	require.NoError(t, DB.Create(task).Error)

	stale := GetStaleTaskSubmissionCheckpoints(
		constant.TaskPlatformAutoDL,
		now-int64((10*time.Minute)/time.Second),
		10,
	)
	assert.Empty(t, stale)
}

func TestUpdateWithStatus_ConcurrentWinner(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID: "task_cas_race",
		Status: TaskStatusInProgress,
		Quota:  1000,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	const goroutines = 5
	wins := make([]bool, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			t := &Task{}
			*t = Task{
				ID:       task.ID,
				TaskID:   task.TaskID,
				Status:   TaskStatusSuccess,
				Progress: "100%",
				Quota:    task.Quota,
				Data:     json.RawMessage(`{}`),
			}
			t.CreatedAt = task.CreatedAt
			t.UpdatedAt = time.Now().Unix()
			won, err := t.UpdateWithStatus(TaskStatusInProgress)
			if err == nil {
				wins[idx] = won
			}
		}(i)
	}
	wg.Wait()

	winCount := 0
	for _, w := range wins {
		if w {
			winCount++
		}
	}
	assert.Equal(t, 1, winCount, "exactly one goroutine should win the CAS")
}
