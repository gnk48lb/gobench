package service

import (
	"errors"
	"gobench/internal/model"
	"gobench/internal/repository"
)

type TaskService interface {
	CreateTask(task *model.Task) error
	GetTask(id uint) (*model.Task, error)
	ListTasks(page, pageSize int) ([]*model.Task, int64, error)
	UpdateTask(task *model.Task) error
	DeleteTask(id uint) error
}

type taskService struct {
	taskRepo repository.TaskRepository
}

func NewTaskService(taskRepo repository.TaskRepository) TaskService {
	return &taskService{taskRepo: taskRepo}
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
		return nil, errors.New("task not found")
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
