package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"gobench/internal/model"
	"gobench/pkg/database"
)

type OverallStats struct {
	TotalLogs     int64   `json:"total_logs"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

type TaskStats struct {
	TaskID        uint       `json:"task_id"`
	TotalRuns     int64      `json:"total_runs"`
	SuccessCount  int64      `json:"success_count"`
	FailedCount   int64      `json:"failed_count"`
	SuccessRate   float64    `json:"success_rate"`
	AvgDurationMs float64    `json:"avg_duration_ms"`
	MaxDurationMs int64      `json:"max_duration_ms"`
	LastRunAt     *time.Time `json:"last_run_at"`
}

type TaskLogRepository interface {
	Create(log *model.TaskLog) error
	GetByID(id uint) (*model.TaskLog, error) // 新增
	UpdateStatus(id uint, workerID string, retryNum int, status, output, errorMsg string, startedAt, finishedAt *time.Time, durationMs int64) error
	ListByTaskID(taskID uint, page, pageSize int) ([]*model.TaskLog, int64, error)
	GetOverallStats(since time.Time) (*OverallStats, error)
	GetTaskStats(taskID uint) (*TaskStats, error)
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

func (r *taskLogRepository) GetByID(id uint) (*model.TaskLog, error) {
	var log model.TaskLog
	err := r.db.First(&log, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &log, nil
}


func (r *taskLogRepository) UpdateStatus(id uint, workerID string, retryNum int, status, output, errorMsg string, startedAt, finishedAt *time.Time, durationMs int64) error {
	updates := map[string]interface{}{
		"worker_id":   workerID,
		"retry_num":   retryNum,
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

func (r *taskLogRepository) GetOverallStats(since time.Time) (*OverallStats, error) {
	var result struct {
		TotalLogs     int64
		SuccessCount  int64
		FailedCount   int64
		AvgDurationMs float64
	}
	err := r.db.Model(&model.TaskLog{}).
		Where("created_at >= ? AND status IN ?", since, []string{"success", "failed"}).
		Select(`
			COUNT(*) as total_logs,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'failed'  THEN 1 ELSE 0 END) as failed_count,
			AVG(duration_ms) as avg_duration_ms
		`).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	stats := &OverallStats{
		TotalLogs:     result.TotalLogs,
		SuccessCount:  result.SuccessCount,
		FailedCount:   result.FailedCount,
		AvgDurationMs: result.AvgDurationMs,
	}
	if result.TotalLogs > 0 {
		stats.SuccessRate = float64(result.SuccessCount) / float64(result.TotalLogs)
	}
	return stats, nil
}

func (r *taskLogRepository) GetTaskStats(taskID uint) (*TaskStats, error) {
	var result struct {
		TotalRuns     int64
		SuccessCount  int64
		FailedCount   int64
		AvgDurationMs float64
		MaxDurationMs int64
		LastRunAt     *time.Time
	}
	err := r.db.Model(&model.TaskLog{}).
		Where("task_id = ? AND status IN ?", taskID, []string{"success", "failed"}).
		Select(`
			COUNT(*) as total_runs,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'failed'  THEN 1 ELSE 0 END) as failed_count,
			AVG(duration_ms) as avg_duration_ms,
			MAX(duration_ms) as max_duration_ms,
			MAX(finished_at) as last_run_at
		`).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	stats := &TaskStats{
		TaskID:        taskID,
		TotalRuns:     result.TotalRuns,
		SuccessCount:  result.SuccessCount,
		FailedCount:   result.FailedCount,
		AvgDurationMs: result.AvgDurationMs,
		MaxDurationMs: result.MaxDurationMs,
		LastRunAt:     result.LastRunAt,
	}
	if result.TotalRuns > 0 {
		stats.SuccessRate = float64(result.SuccessCount) / float64(result.TotalRuns)
	}
	return stats, nil
}
