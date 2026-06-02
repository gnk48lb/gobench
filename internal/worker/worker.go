package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"gobench/internal/model"
	"gobench/internal/queue"
	"gobench/internal/repository"
	"gobench/pkg/config"
	"gobench/pkg/event"
	"gobench/pkg/logger"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Worker struct {
	q           *queue.Queue
	taskRepo    repository.TaskRepository
	logRepo     repository.TaskLogRepository
	stopCh      chan struct{}
	concurrency int
	wg          sync.WaitGroup
	workerID    string
	bus         *event.Bus
}

func NewWorker(q *queue.Queue, taskRepo repository.TaskRepository, logRepo repository.TaskLogRepository, bus *event.Bus) *Worker {
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
		bus:         bus,
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

	// Start delayed queue poller
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.q.MoveDelayedToQueue(context.Background())
			}
		}
	}()
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
			// // 如果队列为空，会阻塞最多2秒后返回redis.Nil错误
			msg, err := w.q.Pop(ctx, 2*time.Second)
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}
				logger.Log.Error("Failed to pop from queue", zap.Error(err), zap.Int("worker_index", workerIndex))
				time.Sleep(1 * time.Second)
				continue
			}

			// 2. Fetch task details BEFORE locking
			task, err := w.taskRepo.GetByID(msg.TaskID)
			if err != nil || task == nil {
				logger.Log.Error("Task not found or failed to fetch", zap.Error(err), zap.Uint("task_id", msg.TaskID))
				continue
			}

			// 3. Process task
			// BRPOP 是原子操作，消息出队的瞬间就从 Redis 里消失，
			// 不可能被其他 worker 重复消费。此处加锁不仅多余，而且在锁竞争失败时
			// 会导致已出队的消息永久丢失。防重复触发的分布式锁应在 scheduler.go 的
			// cron 定时器里使用（那里多实例确实会同时触发），worker 消费队列不需要。
			w.processTask(ctx, msg, task)
		}
	}
}

func (w *Worker) processTask(ctx context.Context, msg *queue.TaskMessage, task *model.Task) {
	now := time.Now()
	// Update status to running
	if err := w.logRepo.UpdateStatus(msg.LogID, w.workerID, msg.RetryNum, "running", "", "", &now, nil, 0); err != nil {
		logger.Log.Error("Failed to update task log to running", zap.Error(err), zap.Uint("log_id", msg.LogID))
		return
	}

	w.bus.Publish(event.Event{
		Type:     event.TypeLogUpdate,
		TaskID:   msg.TaskID,
		LogID:    msg.LogID,
		Status:   "running",
		WorkerID: w.workerID,
	})

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(task.Timeout)*time.Second)
	defer cancel()

	// Execute task
	output, err := ExecuteTask(execCtx, task)
	
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(now).Milliseconds()

	if err != nil {
		// Update as failed
		w.logRepo.UpdateStatus(msg.LogID, w.workerID, msg.RetryNum, "failed", output, err.Error(), nil, &finishedAt, durationMs)

		w.bus.Publish(event.Event{
			Type:       event.TypeLogUpdate,
			TaskID:     msg.TaskID,
			LogID:      msg.LogID,
			Status:     "failed",
			ErrorMsg:   err.Error(),
			DurationMs: durationMs,
			WorkerID:   w.workerID,
		})

		// Retry logic
		if msg.RetryNum < task.RetryCount {
			logger.Log.Info("Retrying task", zap.Uint("task_id", msg.TaskID), zap.Int("retry_num", msg.RetryNum+1))
			
			// Create new pending log for the retry
			retryLog := &model.TaskLog{
				TaskID: task.ID,
				Status: "pending",
			}
			if err := w.logRepo.Create(retryLog); err != nil {
				logger.Log.Error("Failed to create retry task log", zap.Error(err))
				return
			}
			
			retryMsg := queue.TaskMessage{
				TaskID:   msg.TaskID,
				LogID:    retryLog.ID,
				RetryNum: msg.RetryNum + 1,
			}
			if err := w.q.Push(context.Background(), retryMsg); err != nil {
				logger.Log.Error("Failed to push retry message to queue", zap.Error(err))
			}
		}
		return
	}

	// Update as success
	w.logRepo.UpdateStatus(msg.LogID, w.workerID, msg.RetryNum, "success", output, "", nil, &finishedAt, durationMs)

	w.bus.Publish(event.Event{
		Type:       event.TypeLogUpdate,
		TaskID:     msg.TaskID,
		LogID:      msg.LogID,
		Status:     "success",
		Output:     output,
		DurationMs: durationMs,
		WorkerID:   w.workerID,
	})
}

func (w *Worker) Stop() {
	logger.Log.Info("Stopping workers...")
	close(w.stopCh)
	// Wait for all consumption goroutines to finish current tasks
	w.wg.Wait()
	logger.Log.Info("All workers stopped gracefully")
}
