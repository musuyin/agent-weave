package repository

import "time"

type Agent struct {
	ID          string    `gorm:"type:varchar(36);primaryKey"`
	Name        string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	Description string    `gorm:"type:varchar(1000);not null;default:''"`
	Prompt      string    `gorm:"type:mediumtext;not null"`
	IsSystem    bool      `gorm:"not null;default:false"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt   time.Time `gorm:"not null;autoUpdateTime:milli"`
}

func (Agent) TableName() string { return "agents" }

// AgentSkill is the join between an agent and the skills it has loaded.
type AgentSkill struct {
	AgentID string `gorm:"type:varchar(36);primaryKey"`
	SkillID string `gorm:"type:varchar(36);primaryKey"`
}

func (AgentSkill) TableName() string { return "agent_skills" }

// ConversationAgent is the join recording which subagents are present in a chat.
type ConversationAgent struct {
	ConversationID string    `gorm:"type:varchar(36);primaryKey"`
	AgentID        string    `gorm:"type:varchar(36);primaryKey"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime:milli"`
}

func (ConversationAgent) TableName() string { return "conversation_agents" }
