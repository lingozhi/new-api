package image_stream

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type asyncImageWorkerOutcome struct {
	completed bool
	count     bool
	err       error
}

// drainAsyncImageTaskQueue refills each vacant slot as soon as its task finishes.
// Claims remain durable and concurrency bounds both claims and running workers.
func drainAsyncImageTaskQueue(ctx context.Context, concurrency int, execute func(context.Context, *model.Task) (bool, error), afterTask func()) (result asyncImageRunResult, err error) {
	if concurrency < 1 || execute == nil {
		return result, errors.New("image queue requires positive concurrency and an executor")
	}
	refill := time.NewTicker(time.Second)
	defer refill.Stop()
	workers := 0
	outcomes := make(chan asyncImageWorkerOutcome, concurrency)
	var wg sync.WaitGroup
	// A return on cancellation or a database error must not orphan an in-flight
	// worker. The buffered channel lets every outstanding worker report and exit.
	defer wg.Wait()
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if workers < concurrency {
			tasks, err := model.FindPendingImageTasks(concurrency - workers)
			if err != nil {
				return result, fmt.Errorf("find pending image tasks: %w", err)
			}
			for _, task := range tasks {
				if err := ctx.Err(); err != nil {
					return result, err
				}
				claimed, err := model.ClaimImageTask(task, common.GetTimestamp())
				if err != nil {
					return result, fmt.Errorf("claim image task %s: %w", task.TaskID, err)
				}
				if !claimed {
					continue
				}
				workers++
				wg.Add(1)
				go func(task *model.Task) {
					defer wg.Done()
					outcomes <- executeAsyncImageWorkerTask(ctx, task, execute)
				}(task)
			}
		}
		if workers == 0 {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-refill.C:
			// New submissions can arrive while a slower task is still running.
			// Poll vacant slots across instances instead of waiting for that task.
		case outcome := <-outcomes:
			workers--
			if outcome.count {
				if outcome.completed {
					result.Completed++
				} else {
					result.Failed++
				}
			}
			if afterTask != nil {
				afterTask()
			}
			if outcome.err != nil {
				return result, outcome.err
			}
		}
	}
}

// executeAsyncImageWorkerTask preserves settlement recovery and retry handling
// independently of how the queue schedules the next task.
func executeAsyncImageWorkerTask(ctx context.Context, imageTask *model.Task, execute func(context.Context, *model.Task) (bool, error)) (outcome asyncImageWorkerOutcome) {
	completed, err := execute(ctx, imageTask)
	if err != nil {
		if errors.Is(err, errAsyncImageRetryScheduled) {
			return outcome
		}
		if ctx.Err() != nil {
			outcome.err = err
			return outcome
		}
		message := common.MaskSensitiveInfo(err.Error())
		if len(message) > 2000 {
			message = message[:2000]
		}
		if imageTask.Status == model.TaskStatusFinalizing {
			delay := asyncImageRetryDelay(imageTask.FinalizeAttempts)
			if scheduleErr := model.MarkImageTaskFinalizationRetry(imageTask, time.Now().Add(delay).Unix(), message); scheduleErr != nil {
				outcome.err = fmt.Errorf("schedule image task finalization retry %s: %w", imageTask.TaskID, scheduleErr)
			}
			return outcome
		}
		if imageTask.WorkerAttempts+1 >= asyncImageWorkerAttempts {
			if failErr := failAsyncImageTask(ctx, imageTask, fmt.Errorf("image worker exhausted retries: %w", err)); failErr != nil {
				if imageTask.Status == model.TaskStatusFinalizing {
					delay := asyncImageRetryDelay(imageTask.FinalizeAttempts)
					if scheduleErr := model.MarkImageTaskFinalizationRetry(imageTask, time.Now().Add(delay).Unix(), common.MaskSensitiveInfo(failErr.Error())); scheduleErr == nil {
						return outcome
					}
				}
				outcome.err = failErr
				return outcome
			}
			outcome.count = true
			return outcome
		}
		delay := asyncImageRetryDelay(imageTask.WorkerAttempts)
		scheduled, scheduleErr := imageTask.MarkImageWorkerRetry(time.Now().Add(delay).Unix(), message)
		if scheduleErr != nil {
			outcome.err = fmt.Errorf("schedule image worker retry for task %s: %w", imageTask.TaskID, scheduleErr)
			return outcome
		}
		if scheduled {
			logger.LogWarn(ctx, fmt.Sprintf("image worker deferred after unexpected error: task=%s retry=%s err=%s", imageTask.TaskID, delay, message))
		}
		return outcome
	}
	outcome.count = true
	outcome.completed = completed
	return outcome
}
