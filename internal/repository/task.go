package repository

import (
	"errors"
	"gorm.io/gorm"
	"gobench/internal/model"
	"gobench/pkg/database"
)

type TaskRepository interface {
	Create(task *model.Task) error
	GetByID(id uint) (*model.Task, error)
	List(page, pageSize int) ([]*model.Task, int64, error)
	Update(task *model.Task) error
	Delete(id uint) error
}

type taskRepository struct {
	db *gorm.DB
}

func NewTaskRepository() TaskRepository {
	return &taskRepository{db: database.DB}
}

func (r *taskRepository) Create(task *model.Task) error {
	return r.db.Create(task).Error
}

func (r *taskRepository) GetByID(id uint) (*model.Task, error) {
	var task model.Task
	err := r.db.First(&task, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func (r *taskRepository) List(page, pageSize int) ([]*model.Task, int64, error) {
	var tasks []*model.Task
	var total int64

	// Count total records
	err := r.db.Model(&model.Task{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Fetch paginated records
	offset := (page - 1) * pageSize
	err = r.db.Offset(offset).Limit(pageSize).Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func (r *taskRepository) Update(task *model.Task) error {
	return r.db.Save(task).Error
}

func (r *taskRepository) Delete(id uint) error {
	return r.db.Delete(&model.Task{}, id).Error
}
