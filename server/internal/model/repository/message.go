package repository

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ContentBlock is one element of a message's content array.
// Type is one of: "text", "tool_use", "tool_result".
type ContentBlock struct {
	Type string `json:"type"`

	// text fields
	Text string `json:"text,omitempty"`

	// tool_use fields
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
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
	AgentID        *string       `gorm:"type:varchar(36)"`          // producing agent; NULL = orchestrator
	Content        ContentBlocks `gorm:"type:json;not null"`
	CreatedAt      time.Time     `gorm:"not null;autoCreateTime:milli"`
}

func (Message) TableName() string { return "messages" }
