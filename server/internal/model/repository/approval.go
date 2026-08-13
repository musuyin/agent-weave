package repository

import "time"

// Approval records a pending or decided approval for a high-risk tool call.
// BlockID (the Anthropic tool_use block ID) serves as the natural primary key.
type Approval struct {
	BlockID        string     `gorm:"primaryKey"`
	ConversationID string     `gorm:"index"`
	ToolName       string
	Status         string // pending / approved / rejected
	CreatedAt      time.Time
	DecidedAt      *time.Time
}
