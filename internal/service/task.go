package service

import (
	"time"

	"gorm.io/gorm"

	"gobench/internal/model"
	"gobench/internal/repository"
	"gobench/pkg/apperrors"
)

// TaskExecutor 是 taskService 依赖的执行器接口，由 grpcclient.ExecutorClient 实现
type TaskExecutor interface {
	TriggerTask(taskID uint) (uint, error)
	ScheduleTask(taskID uint, delaySeconds int) (uint, error)
}

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
	executor TaskExecutor
}

func NewTaskService(
	taskRepo repository.TaskRepository,
	logRepo repository.TaskLogRepository,
	executor TaskExecutor,
) TaskService {
	return &taskService{
		taskRepo: taskRepo,
		logRepo:  logRepo,
		executor: executor,
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
	// 仍在 api-service 侧验证 task 存在，executor 信任传入的 task_id
	_, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	logID, err := s.executor.TriggerTask(taskID)
	if err != nil {
		return nil, err
	}

	log, err := s.logRepo.GetByID(logID)
	if err != nil {
		return nil, err
	}
	if log == nil {
		return &model.TaskLog{
			Model:  gorm.Model{ID: logID},
			TaskID: taskID,
			Status: "pending",
		}, nil
	}
	return log, nil
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
	_, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	logID, err := s.executor.ScheduleTask(taskID, delaySeconds)
	if err != nil {
		return nil, err
	}

	log, err := s.logRepo.GetByID(logID)
	if err != nil {
		return nil, err
	}
	if log == nil {
		return &model.TaskLog{
			Model:  gorm.Model{ID: logID},
			TaskID: taskID,
			Status: "pending",
		}, nil
	}
	return log, nil
}

func (s *taskService) GetOverallStats(since time.Time) (*repository.OverallStats, error) {
	return s.logRepo.GetOverallStats(since)
}

func (s *taskService) GetTaskStats(taskID uint) (*repository.TaskStats, error) {
	return s.logRepo.GetTaskStats(taskID)
}
