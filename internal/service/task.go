package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gobench/internal/model"
	"gobench/internal/queue"
	"gobench/internal/repository"
	"gobench/pkg/apperrors"
)

type TaskService interface {
	CreateTask(task *model.Task) error
	GetTask(id uint) (*model.Task, error)
	ListTasks(page, pageSize int) ([]*model.Task, int64, error)
	UpdateTask(task *model.Task) error
	DeleteTask(id uint) error
	TriggerTask(taskID uint) (*model.TaskLog, error)
	GetTaskLogs(taskID uint, page, pageSize int) ([]*model.TaskLog, int64, error)
	ScheduleTask(taskID uint, delaySeconds int) (*model.TaskLog, error)
	GetOverallStats(since time.Time) (*repository.OverallStats, error)
	GetTaskStats(taskID uint) (*repository.TaskStats, error)
}

type taskService struct {
	taskRepo repository.TaskRepository
	logRepo  repository.TaskLogRepository
	q        *queue.Queue
}

func NewTaskService(taskRepo repository.TaskRepository, logRepo repository.TaskLogRepository, q *queue.Queue) TaskService {
	return &taskService{
		taskRepo: taskRepo,
		logRepo:  logRepo,
		q:        q,
	}
}

func (s *taskService) CreateTask(task *model.Task) error {
	// Add business logic here if needed (e.g., validate cron expr)
	return s.taskRepo.Create(task)
}

func (s *taskService) GetTask(id uint) (*model.Task, error) {
	task, err := s.taskRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, apperrors.ErrTaskNotFound
	}
	return task, nil
}

func (s *taskService) ListTasks(page, pageSize int) ([]*model.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return s.taskRepo.List(page, pageSize)
}

func (s *taskService) UpdateTask(task *model.Task) error {
	// Add check if task exists
	existing, err := s.GetTask(task.ID)
	if err != nil {
		return err
	}

	// Update allowed fields
	existing.Name = task.Name
	existing.TaskType = task.TaskType
	existing.CronExpr = task.CronExpr
	existing.Payload = task.Payload
	existing.RetryCount = task.RetryCount
	existing.Timeout = task.Timeout
	existing.Status = task.Status

	return s.taskRepo.Update(existing)
}

func (s *taskService) DeleteTask(id uint) error {
	// Check if exists
	_, err := s.GetTask(id)
	if err != nil {
		return err
	}
	return s.taskRepo.Delete(id)
}

func (s *taskService) TriggerTask(taskID uint) (*model.TaskLog, error) {
	// 1. Validate task exists
	_, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	// 2. Create pending log
	taskLog := &model.TaskLog{
		TaskID: taskID,
		Status: "pending",
	}
	if err := s.logRepo.Create(taskLog); err != nil {
		return nil, err
	}

	// 3. Push to queue
	msg := queue.TaskMessage{
		TaskID: taskID,
		LogID:  taskLog.ID,
	}
	if err := s.q.Push(context.Background(), msg); err != nil {
		// 补偿：入队失败时把日志标记为 failed，避免产生永久 pending 的孤儿记录
		now := time.Now()
		_ = s.logRepo.UpdateStatus(taskLog.ID, "", 0, "failed", "", "failed to enqueue: "+err.Error(), &now, &now, 0)
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	return taskLog, nil
}

func (s *taskService) GetTaskLogs(taskID uint, page, pageSize int) ([]*model.TaskLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return s.logRepo.ListByTaskID(taskID, page, pageSize)
}

func (s *taskService) ScheduleTask(taskID uint, delaySeconds int) (*model.TaskLog, error) {
	// 1. Validate task exists
	_, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	// 2. Create pending log
	taskLog := &model.TaskLog{
		TaskID: taskID,
		Status: "pending", // Scheduled tasks are also pending until worker picks them up
	}
	if err := s.logRepo.Create(taskLog); err != nil {
		return nil, err
	}

	// 3. Push to delayed queue
	msg := queue.TaskMessage{
		TaskID: taskID,
		LogID:  taskLog.ID,
	}
	runAt := time.Now().Add(time.Duration(delaySeconds) * time.Second)
	if err := s.q.PushDelayed(context.Background(), msg, runAt); err != nil {
		return nil, errors.New("failed to schedule task")
	}

	return taskLog, nil
}

func (s *taskService) GetOverallStats(since time.Time) (*repository.OverallStats, error) {
	return s.logRepo.GetOverallStats(since)
}

func (s *taskService) GetTaskStats(taskID uint) (*repository.TaskStats, error) {
	return s.logRepo.GetTaskStats(taskID)
}
