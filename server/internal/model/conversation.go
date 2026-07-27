package model

import "time"

type Conversation struct {
	ID        string    `gorm:"type:varchar(36);primaryKey"`
	Title     string    `gorm:"type:varchar(500);not null;default:''"`
	CreatedAt time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt time.Time `gorm:"type:datetime(3);not null;autoUpdateTime:milli"`
}

func (Conversation) TableName() string { return "conversations" }
