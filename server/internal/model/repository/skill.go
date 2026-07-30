package repository

import "time"

type Skill struct {
	ID          string    `gorm:"type:varchar(36);primaryKey"`
	Name        string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	Description string    `gorm:"type:varchar(1000);not null;default:''"`
	Body        string    `gorm:"type:mediumtext;not null"`
	IsSystem    bool      `gorm:"not null;default:false"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt   time.Time `gorm:"not null;autoUpdateTime:milli"`
}

func (Skill) TableName() string { return "skills" }
