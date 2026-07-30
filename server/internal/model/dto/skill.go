package dto

type CreateSkillRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Body        string `json:"body" binding:"required"`
}

type UpdateSkillRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Body        string `json:"body" binding:"required"`
}
