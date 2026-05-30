package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gobench/internal/queue"
	"gobench/internal/repository"
	"gobench/pkg/config"
	"gobench/pkg/logger"
	pkgredis "gobench/pkg/redis"
)

type Worker struct {
	q           *queue.Queue
	taskRepo    repository.TaskRepository
	logRepo     repository.TaskLogRepository
	stopCh      chan struct{}
	concurrency int
	wg          sync.WaitGroup
	workerID    string
}

func NewWorker(q *queue.Queue, taskRepo repository.TaskRepository, logRepo repository.TaskLogRepository) *Worker {
	concurrency := 5
	if config.AppConfig != nil && config.AppConfig.Worker.Concurrency > 0 {
		concurrency = config.AppConfig.Worker.Concurrency
	}

	return &Worker{
		q:           q,
		taskRepo:    taskRepo,
		logRepo:     logRepo,
		stopCh:      make(chan struct{}),
		concurrency: concurrency,
		workerID:    uuid.New().String(),
	}
}

func (w *Worker) Start() {
	logger.Log.Info("Starting workers...", zap.Int("concurrency", w.concurrency), zap.String("worker_id", w.workerID))
	
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go func(workerIndex int) {
			defer w.wg.Done()
			w.loop(workerIndex)
		}(i)
	}
}

func (w *Worker) loop(workerIndex int) {
	logger.Log.Info("Worker thread started", zap.Int("worker_index", workerIndex))
	for {
		select {
		case <-w.stopCh:
			logger.Log.Info("Worker thread stopped", zap.Int("worker_index", workerIndex))
			return
		default:
			ctx := context.Background()
			
			// 1. Pop message (timeout 2s)
			msg, err := w.q.Pop(ctx, 2*time.Second)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}
				logger.Log.Error("Failed to pop from queue", zap.Error(err), zap.Int("worker_index", workerIndex))
				time.Sleep(1 * time.Second)
				continue
			}

			// 2. Distributed lock to prevent split-brain
			lockKey := fmt.Sprintf("gobench:lock:task:%d", msg.TaskID)
			acquired, err := pkgredis.AcquireLock(ctx, pkgredis.Client, lockKey, w.workerID, 5*time.Minute)
			if err != nil {
				logger.Log.Error("Failed to acquire lock", zap.Error(err), zap.Uint("task_id", msg.TaskID))
				continue
			}
			if !acquired {
				logger.Log.Info("Lock not acquired, task handled by another worker", zap.Uint("task_id", msg.TaskID))
				continue
			}

			// 3. Guarantee lock release
			w.processTask(ctx, msg, lockKey)
		}
	}
}

func (w *Worker) processTask(ctx context.Context, msg *queue.TaskMessage, lockKey string) {
	// Defer lock release
	defer pkgredis.ReleaseLock(ctx, pkgredis.Client, lockKey, w.workerID)

	now := time.Now()
	// Update status to running
	if err := w.logRepo.UpdateStatus(msg.LogID, "running", "", "", &now, nil, 0); err != nil {
		logger.Log.Error("Failed to update task log to running", zap.Error(err), zap.Uint("log_id", msg.LogID))
		return
	}

	// Fetch task details
	task, err := w.taskRepo.GetByID(msg.TaskID)
	if err != nil || task == nil {
		errorMsg := "Task not found or failed to fetch"
		if err != nil {
			errorMsg = err.Error()
		}
		finishedAt := time.Now()
		durationMs := finishedAt.Sub(now).Milliseconds()
		w.logRepo.UpdateStatus(msg.LogID, "failed", "", errorMsg, nil, &finishedAt, durationMs)
		return
	}

	// Execute task
	output, err := ExecuteTask(ctx, task)
	
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(now).Milliseconds()

	if err != nil {
		// Update as failed
		w.logRepo.UpdateStatus(msg.LogID, "failed", output, err.Error(), nil, &finishedAt, durationMs)
		return
	}

	// Update as success
	w.logRepo.UpdateStatus(msg.LogID, "success", output, "", nil, &finishedAt, durationMs)
}

func (w *Worker) Stop() {
	logger.Log.Info("Stopping workers...")
	close(w.stopCh)
	// Wait for all consumption goroutines to finish current tasks
	w.wg.Wait()
	logger.Log.Info("All workers stopped gracefully")
}
