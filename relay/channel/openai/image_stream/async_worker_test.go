package image_stream

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageQueueRefillsSlotBeforeOtherTaskFinishes(t *testing.T) {
	setupAsyncImageSubmitTestDB(t)
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		require.NoError(t, model.DB.Create(&model.Task{TaskID: fmt.Sprintf("task_slot_%d", i), Platform: constant.TaskPlatformOpenAIImage, Status: model.TaskStatusNotStart, SubmitTime: now, CreatedAt: now, UpdatedAt: now}).Error)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 3)
	releases := map[string]chan struct{}{}
	for i := 0; i < 3; i++ {
		releases[fmt.Sprintf("task_slot_%d", i)] = make(chan struct{}, 1)
	}
	finished := make(chan struct{})
	var result asyncImageRunResult
	var queueErr error
	go func() {
		defer close(finished)
		result, queueErr = drainAsyncImageTaskQueue(ctx, 2, func(ctx context.Context, task *model.Task) (bool, error) {
			started <- task.TaskID
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-releases[task.TaskID]:
			}
			err := model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusSuccess).Error
			return err == nil, err
		}, nil)
	}()
	defer func() { cancel(); <-finished }()
	var first, second string
	select {
	case first = <-started:
	case <-ctx.Done():
		t.Fatal("first worker did not start")
	}
	select {
	case second = <-started:
	case <-ctx.Done():
		t.Fatal("second worker did not start")
	}
	assert.NotEqual(t, first, second)
	select {
	case unexpected := <-started:
		t.Fatalf("concurrency exceeded: %s", unexpected)
	default:
	}
	releases[first] <- struct{}{}
	var third string
	select {
	case third = <-started:
	case <-ctx.Done():
		t.Fatal("idle slot waited for the blocked task instead of taking queued work")
	}
	assert.NotContains(t, []string{first, second}, third)
	releases[second] <- struct{}{}
	releases[third] <- struct{}{}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("queue did not drain")
	}
	require.NoError(t, queueErr)
	assert.Equal(t, 3, result.Completed)
	assert.Zero(t, result.Failed)
	var tasks []model.Task
	require.NoError(t, model.DB.Find(&tasks).Error)
	for _, task := range tasks {
		assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
		assert.Equal(t, 1, task.Attempt)
	}
}

func TestImageQueueCancellationJoinsWorkersWithoutClaimingMoreTasks(t *testing.T) {
	setupAsyncImageSubmitTestDB(t)
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		require.NoError(t, model.DB.Create(&model.Task{TaskID: fmt.Sprintf("task_cancel_%d", i), Platform: constant.TaskPlatformOpenAIImage, Status: model.TaskStatusNotStart, SubmitTime: now, CreatedAt: now, UpdatedAt: now}).Error)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	var exited sync.WaitGroup
	exited.Add(2)
	finished := make(chan error, 1)
	go func() {
		_, err := drainAsyncImageTaskQueue(ctx, 2, func(ctx context.Context, _ *model.Task) (bool, error) {
			defer exited.Done()
			started <- struct{}{}
			<-ctx.Done()
			return false, ctx.Err()
		}, nil)
		finished <- err
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-deadline.C:
			cancel()
			<-finished
			t.Fatal("worker did not start")
		}
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-deadline.C:
		t.Fatal("canceled queue did not join workers")
	}
	exited.Wait()
	var pending int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("status = ? AND attempt = ?", model.TaskStatusNotStart, 0).Count(&pending).Error)
	assert.Equal(t, int64(1), pending)
}

func TestImageQueueStartsNewArrivalWhileExistingTaskIsBlocked(t *testing.T) {
	setupAsyncImageSubmitTestDB(t)
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_first_arrival", Platform: constant.TaskPlatformOpenAIImage, Status: model.TaskStatusNotStart, SubmitTime: now, CreatedAt: now, UpdatedAt: now}).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 2)
	finished := make(chan error, 1)
	go func() {
		_, err := drainAsyncImageTaskQueue(ctx, 2, func(ctx context.Context, task *model.Task) (bool, error) {
			started <- task.TaskID
			<-ctx.Done()
			return false, ctx.Err()
		}, nil)
		finished <- err
	}()
	defer func() { cancel(); <-finished }()
	select {
	case id := <-started:
		assert.Equal(t, "task_first_arrival", id)
	case <-ctx.Done():
		t.Fatal("initial task did not start")
	}
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_later_arrival", Platform: constant.TaskPlatformOpenAIImage, Status: model.TaskStatusNotStart, SubmitTime: now, CreatedAt: now, UpdatedAt: now}).Error)
	select {
	case id := <-started:
		assert.Equal(t, "task_later_arrival", id)
	case <-ctx.Done():
		t.Fatal("new arrival could not use an idle slot while existing work was blocked")
	}
}
