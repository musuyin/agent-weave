package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/seeding"
)

type AgentService struct {
	db *gorm.DB
}

func NewAgentService(db *gorm.DB) *AgentService {
	return &AgentService{db: db}
}

// ProvideAgentService constructs the service and seeds system defaults at startup.
// Distinct from agent.ProvideAgentService (different package).
func ProvideAgentService(ctx context.Context, db *gorm.DB) (*AgentService, error) {
	s := NewAgentService(db)
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// List returns all agents ordered by name.
func (s *AgentService) List(ctx context.Context) ([]repository.Agent, error) {
	var agents []repository.Agent
	err := s.db.WithContext(ctx).Order("name ASC").Find(&agents).Error
	return agents, err
}

// Get returns the agent with the given id, or ErrNotFound.
func (s *AgentService) Get(ctx context.Context, id string) (repository.Agent, error) {
	var agent repository.Agent
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agent, ErrNotFound
	}
	return agent, err
}

// Create persists a new user agent.
func (s *AgentService) Create(ctx context.Context, name, description, prompt string) (repository.Agent, error) {
	now := time.Now().UTC()
	agent := repository.Agent{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Prompt:      prompt,
		IsSystem:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return agent, s.db.WithContext(ctx).Create(&agent).Error
}

// Update mutates a user agent. System agents are read-only.
func (s *AgentService) Update(ctx context.Context, id, name, description, prompt string) (repository.Agent, error) {
	agent, err := s.Get(ctx, id)
	if err != nil {
		return agent, err
	}
	if agent.IsSystem {
		return agent, ErrSystemReadOnly
	}
	agent.Name = name
	agent.Description = description
	agent.Prompt = prompt
	agent.UpdatedAt = time.Now().UTC()
	return agent, s.db.WithContext(ctx).Save(&agent).Error
}

// Delete removes a user agent. System agents are read-only.
func (s *AgentService) Delete(ctx context.Context, id string) error {
	agent, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if agent.IsSystem {
		return ErrSystemReadOnly
	}
	return s.db.WithContext(ctx).Delete(&repository.Agent{}, "id = ?", id).Error
}

// ListSkills returns the skills loaded on an agent.
func (s *AgentService) ListSkills(ctx context.Context, agentID string) ([]repository.Skill, error) {
	if _, err := s.Get(ctx, agentID); err != nil {
		return nil, err
	}
	var skills []repository.Skill
	err := s.db.WithContext(ctx).
		Joins("JOIN agent_skills ON agent_skills.skill_id = skills.id").
		Where("agent_skills.agent_id = ?", agentID).
		Order("skills.name ASC").
		Find(&skills).Error
	return skills, err
}

// LoadSkill attaches a skill to an agent. Idempotent (duplicate join is ignored).
func (s *AgentService) LoadSkill(ctx context.Context, agentID, skillID string) error {
	if _, err := s.Get(ctx, agentID); err != nil {
		return err
	}
	var skill repository.Skill
	if err := s.db.WithContext(ctx).Where("id = ?", skillID).First(&skill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	link := repository.AgentSkill{AgentID: agentID, SkillID: skillID}
	err := s.db.WithContext(ctx).Create(&link).Error
	if err != nil && isDuplicateKeyError(err) {
		return nil
	}
	return err
}

// UnloadSkill detaches a skill from an agent.
func (s *AgentService) UnloadSkill(ctx context.Context, agentID, skillID string) error {
	return s.db.WithContext(ctx).
		Where("agent_id = ? AND skill_id = ?", agentID, skillID).
		Delete(&repository.AgentSkill{}).Error
}

// ListConversationAgents returns the subagents present in a conversation.
func (s *AgentService) ListConversationAgents(ctx context.Context, convID string) ([]repository.Agent, error) {
	var agents []repository.Agent
	err := s.db.WithContext(ctx).
		Joins("JOIN conversation_agents ON conversation_agents.agent_id = agents.id").
		Where("conversation_agents.conversation_id = ?", convID).
		Order("agents.name ASC").
		Find(&agents).Error
	return agents, err
}

// AddAgentToConversation adds a subagent to a chat. Idempotent.
func (s *AgentService) AddAgentToConversation(ctx context.Context, convID, agentID string) error {
	if _, err := s.Get(ctx, agentID); err != nil {
		return err
	}
	link := repository.ConversationAgent{
		ConversationID: convID,
		AgentID:        agentID,
		CreatedAt:      time.Now().UTC(),
	}
	err := s.db.WithContext(ctx).Create(&link).Error
	if err != nil && isDuplicateKeyError(err) {
		return nil
	}
	return err
}

// RemoveAgentFromConversation removes a subagent from a chat.
func (s *AgentService) RemoveAgentFromConversation(ctx context.Context, convID, agentID string) error {
	return s.db.WithContext(ctx).
		Where("conversation_id = ? AND agent_id = ?", convID, agentID).
		Delete(&repository.ConversationAgent{}).Error
}

// getOrCreateSystemAgent find-or-creates a system agent by unique name, returning its id.
func (s *AgentService) getOrCreateSystemAgent(ctx context.Context, name, description, prompt string) (string, error) {
	var existing repository.Agent
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	now := time.Now().UTC()
	agent := repository.Agent{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Prompt:      prompt,
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if createErr := s.db.WithContext(ctx).Create(&agent).Error; createErr != nil {
		if !isDuplicateKeyError(createErr) {
			return "", createErr
		}
		if err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; err != nil {
			return "", err
		}
		return existing.ID, nil
	}
	return agent.ID, nil
}

// findSkillIDByName returns the id of a skill by name, or ("", nil) if absent.
func (s *AgentService) findSkillIDByName(ctx context.Context, name string) (string, error) {
	var skill repository.Skill
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return skill.ID, nil
}

// Init idempotently seeds system default agents and their skill loadouts from
// the embedded seeding data. SkillService.Init must have run first.
func (s *AgentService) Init(ctx context.Context) error {
	for _, def := range seeding.SystemAgents {
		agentID, err := s.getOrCreateSystemAgent(ctx, def.Name, def.Description, def.Prompt)
		if err != nil {
			return err
		}
		for _, skillName := range def.Skills {
			skillID, err := s.findSkillIDByName(ctx, skillName)
			if err != nil {
				return err
			}
			if skillID == "" {
				continue
			}
			if err := s.LoadSkill(ctx, agentID, skillID); err != nil {
				return err
			}
		}
	}
	return nil
}
