package dto

type CreateAgentRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Prompt      string `json:"prompt" binding:"required"`
}

type UpdateAgentRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Prompt      string `json:"prompt" binding:"required"`
}

type LoadSkillRequest struct {
	SkillID string `json:"skill_id" binding:"required"`
}

type AddAgentRequest struct {
	AgentID string `json:"agent_id" binding:"required"`
}
