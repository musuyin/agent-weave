package repository

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type ThreadStatus string

const (
	ThreadStatusPending   ThreadStatus = "pending"
	ThreadStatusRunning   ThreadStatus = "running"
	ThreadStatusDone      ThreadStatus = "done"
	ThreadStatusCancelled ThreadStatus = "cancelled"
	ThreadStatusError     ThreadStatus = "error"
)

// StringSlice is a JSON-serialisable []string used for threads.blocked_by.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *StringSlice) Scan(value any) error {
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("StringSlice.Scan: unsupported type %T", value)
	}
	return json.Unmarshal(b, s)
}

type Thread struct {
	ID             string       `gorm:"type:varchar(36);primaryKey"`
	ConversationID string       `gorm:"type:varchar(36);not null"`
	AgentID        string       `gorm:"type:varchar(255);not null"`
	Status         ThreadStatus `gorm:"type:varchar(50);not null;default:pending"`
	BlockedBy      StringSlice  `gorm:"type:json;not null"`
	CreatedAt      time.Time    `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt      time.Time    `gorm:"not null;autoUpdateTime:milli"`
}

func (Thread) TableName() string { return "threads" }
