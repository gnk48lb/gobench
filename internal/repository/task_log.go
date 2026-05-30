package repository

import (
	"gorm.io/gorm"
	"time"
	"gobench/internal/model"
	"gobench/pkg/database"
)

type TaskLogRepository interface {
	Create(log *model.TaskLog) error
	UpdateStatus(id uint, status, output, errorMsg string, startedAt, finishedAt *time.Time, durationMs int64) error
	ListByTaskID(taskID uint, page, pageSize int) ([]*model.TaskLog, int64, error)
}

type taskLogRepository struct {
	db *gorm.DB
}

func NewTaskLogRepository() TaskLogRepository {
	return &taskLogRepository{db: database.DB}
}

func (r *taskLogRepository) Create(log *model.TaskLog) error {
	return r.db.Create(log).Error
}

func (r *taskLogRepository) UpdateStatus(id uint, status, output, errorMsg string, startedAt, finishedAt *time.Time, durationMs int64) error {
	updates := map[string]interface{}{
		"status":      status,
		"output":      output,
		"error_msg":   errorMsg,
		"duration_ms": durationMs,
	}
	if startedAt != nil {
		updates["started_at"] = startedAt
	}
	if finishedAt != nil {
		updates["finished_at"] = finishedAt
	}
	return r.db.Model(&model.TaskLog{}).Where("id = ?", id).Updates(updates).Error
}

func (r *taskLogRepository) ListByTaskID(taskID uint, page, pageSize int) ([]*model.TaskLog, int64, error) {
	var logs []*model.TaskLog
	var total int64

	query := r.db.Model(&model.TaskLog{}).Where("task_id = ?", taskID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
