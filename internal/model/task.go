package model

import (
	"gorm.io/gorm"
)

type Task struct {
	gorm.Model
	Name         string `gorm:"type:varchar(100);not null;uniqueIndex" json:"name"` // 任务名称
	TaskType     string `gorm:"type:varchar(50);not null" json:"task_type"`         // 任务类型 (如: http, shell, function)
	CronExpr     string `gorm:"type:varchar(100)" json:"cron_expr"`                 // Cron 表达式
	Payload      string `gorm:"type:text" json:"payload"`                           // 任务参数 (JSON格式，供Worker解析)
	RetryCount   int    `gorm:"default:0" json:"retry_count"`                       // 允许重试次数
	Timeout      int    `gorm:"default:60" json:"timeout"`                          // 任务超时时间(秒)
	Status       string `gorm:"type:varchar(20);default:'active'" json:"status"`    // 状态: active, paused, deleted
	CreatorID    uint   `json:"creator_id"`                                         // 创建者ID (关联User)
}
