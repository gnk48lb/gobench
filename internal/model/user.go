package model

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username string `gorm:"type:varchar(100);not null;uniqueIndex" json:"username"`
	Password string `gorm:"type:varchar(255);not null" json:"-"` // Omit password in JSON
	Role     string `gorm:"type:varchar(20);default:'user'" json:"role"`
}
