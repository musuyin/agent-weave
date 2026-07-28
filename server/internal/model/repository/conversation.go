package repository

import "time"

type Conversation struct {
	ID        string    `gorm:"type:varchar(36);primaryKey"`
	Title     string    `gorm:"type:varchar(500);not null;default:''"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime:milli"`
}

func (Conversation) TableName() string { return "conversations" }
