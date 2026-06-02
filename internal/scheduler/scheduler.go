package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gobench/internal/model"
	"gobench/pkg/logger"
	redispkg "gobench/pkg/redis"
)

// TaskTriggerer 是调度器依赖的接口，由 TaskService 实现
// 定义在这里避免 import cycle
type TaskTriggerer interface {
	TriggerTask(taskID uint) (*model.TaskLog, error)
}

type Scheduler struct {
	c         *cron.Cron
	entryMap  map[uint]cron.EntryID // taskID → cron entry ID
	mu        sync.Mutex
	triggerer TaskTriggerer
}

func NewScheduler(triggerer TaskTriggerer) *Scheduler {
	return &Scheduler{
		c:         cron.New(cron.WithSeconds()), // 支持秒级，格式: 秒 分 时 日 月 周
		entryMap:  make(map[uint]cron.EntryID),
		triggerer: triggerer,
	}
}

func (s *Scheduler) Start() {
	s.c.Start()
	logger.Log.Info("Cron scheduler started")
}

func (s *Scheduler) Stop() {
	ctx := s.c.Stop()
	<-ctx.Done() // 等待所有正在运行的 cron job 完成
	logger.Log.Info("Cron scheduler stopped")
}

// RegisterTask 注册一个任务到调度器，仅当 task.CronExpr 非空且 task.Status=="active" 时生效
func (s *Scheduler) RegisterTask(task *model.Task) error {
	if task.CronExpr == "" || task.Status != "active" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已注册过，先删除旧的
	if entryID, ok := s.entryMap[task.ID]; ok {
		s.c.Remove(entryID)
	}

	taskID := task.ID
	taskName := task.Name
	entryID, err := s.c.AddFunc(task.CronExpr, func() {
		// 尝试抢分布式锁，防止多实例重复触发
		lockKey := fmt.Sprintf("gobench:cron:fire:%d:%d", taskID, time.Now().Unix()/60)
		acquired, err := redispkg.AcquireLock(
			context.Background(), redispkg.Client, lockKey, "1", 50*time.Second,
		)
		if err != nil || !acquired {
			return // 其他实例已经触发了
		}

		log, err := s.triggerer.TriggerTask(taskID)
		if err != nil {
			logger.Log.Error("Cron trigger failed",
				zap.Uint("task_id", taskID),
				zap.String("task_name", taskName),
				zap.Error(err),
			)
			return
		}
		logger.Log.Info("Cron triggered task",
			zap.Uint("task_id", taskID),
			zap.String("task_name", taskName),
			zap.Uint("log_id", log.ID),
		)
	})
	if err != nil {
		return fmt.Errorf("invalid cron expr '%s': %w", task.CronExpr, err)
	}

	s.entryMap[task.ID] = entryID
	logger.Log.Info("Registered cron task",
		zap.Uint("task_id", task.ID),
		zap.String("cron_expr", task.CronExpr),
	)
	return nil
}

// UnregisterTask 从调度器中移除任务
func (s *Scheduler) UnregisterTask(taskID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entryMap[taskID]; ok {
		s.c.Remove(entryID)
		delete(s.entryMap, taskID)
		logger.Log.Info("Unregistered cron task", zap.Uint("task_id", taskID))
	}
}

// UpdateTask 更新调度计划（先删除再注册）
func (s *Scheduler) UpdateTask(task *model.Task) error {
	s.UnregisterTask(task.ID)
	return s.RegisterTask(task)
}

// LoadActiveTasks 启动时从数据库加载所有 active 且有 cron 表达式的任务
func (s *Scheduler) LoadActiveTasks(tasks []*model.Task) {
	for _, task := range tasks {
		if err := s.RegisterTask(task); err != nil {
			logger.Log.Warn("Failed to register cron task on startup",
				zap.Uint("task_id", task.ID),
				zap.Error(err),
			)
		}
	}
	logger.Log.Info("Loaded active cron tasks", zap.Int("count", len(s.entryMap)))
}
