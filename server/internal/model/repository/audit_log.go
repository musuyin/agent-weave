package repository

import "time"

// AuditLog records a tool call outcome for structured audit trails.
// Param values are never stored — only the key names (invariant H).
type AuditLog struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	ConversationID string    `gorm:"index"`
	ToolName       string    `gorm:"index"`
	ParamKeys      string    // JSON array of parameter key names
	Success        bool
	ErrorMessage   string
	CreatedAt      time.Time
}
