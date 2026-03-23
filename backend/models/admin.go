package models

import (
	"time"
)

// Admin 管理员
type Admin struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"type:varchar(255);not null;column:password_hash" json:"-"`
	Email        string     `gorm:"type:varchar(100)" json:"email"`
	Role         string     `gorm:"type:varchar(20);default:admin" json:"role"`
	Status       int8       `gorm:"type:tinyint;default:1" json:"status"`
	LastLoginAt  *time.Time `gorm:"precision:0" json:"last_login_at"`
	CreatedAt    time.Time  `gorm:"precision:0" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
}

func (Admin) TableName() string {
	return "admins"
}
