package repository

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ContentBlock is one element of a message's content array.
// Type: "text" | "tool_use" | "tool_result" (Phase 1 will extend this).
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ContentBlocks is a JSON-serialisable slice stored in messages.content.
type ContentBlocks []ContentBlock

func (c ContentBlocks) Value() (driver.Value, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

func (c *ContentBlocks) Scan(value any) error {
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("ContentBlocks.Scan: unsupported type %T", value)
	}
	return json.Unmarshal(b, c)
}

type Message struct {
	ID             string        `gorm:"type:varchar(36);primaryKey"`
	ConversationID string        `gorm:"type:varchar(36);not null"`
	Role           string        `gorm:"type:varchar(20);not null"` // "user" | "assistant"
	Content        ContentBlocks `gorm:"type:json;not null"`
	CreatedAt      time.Time     `gorm:"not null;autoCreateTime:milli"`
}

func (Message) TableName() string { return "messages" }
