package model

import (
	"gorm.io/gorm"
	"time"
)

type TaskLog struct {
	gorm.Model
	TaskID     uint       `gorm:"index;not null" json:"task_id"`
	WorkerID   string     `gorm:"type:varchar(100)" json:"worker_id"`
	RetryNum   int        `gorm:"default:0" json:"retry_num"`
	Status     string     `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending, running, success, failed
	Output     string     `gorm:"type:text" json:"output"`
	ErrorMsg   string     `gorm:"type:text" json:"error_msg"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs int64      `json:"duration_ms"`
}
