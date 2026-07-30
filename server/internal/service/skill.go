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

type SkillService struct {
	db *gorm.DB
}

func NewSkillService(db *gorm.DB) *SkillService {
	return &SkillService{db: db}
}

// ProvideSkillService constructs the service and seeds system defaults at startup.
func ProvideSkillService(ctx context.Context, db *gorm.DB) (*SkillService, error) {
	s := NewSkillService(db)
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// List returns all skills ordered by name.
func (s *SkillService) List(ctx context.Context) ([]repository.Skill, error) {
	var skills []repository.Skill
	err := s.db.WithContext(ctx).Order("name ASC").Find(&skills).Error
	return skills, err
}

// Get returns the skill with the given id, or ErrNotFound.
func (s *SkillService) Get(ctx context.Context, id string) (repository.Skill, error) {
	var skill repository.Skill
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return skill, ErrNotFound
	}
	return skill, err
}

// Create persists a new user skill.
func (s *SkillService) Create(ctx context.Context, name, description, body string) (repository.Skill, error) {
	now := time.Now().UTC()
	skill := repository.Skill{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Body:        body,
		IsSystem:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return skill, s.db.WithContext(ctx).Create(&skill).Error
}

// Update mutates a user skill. System skills are read-only.
func (s *SkillService) Update(ctx context.Context, id, name, description, body string) (repository.Skill, error) {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return skill, err
	}
	if skill.IsSystem {
		return skill, ErrSystemReadOnly
	}
	skill.Name = name
	skill.Description = description
	skill.Body = body
	skill.UpdatedAt = time.Now().UTC()
	return skill, s.db.WithContext(ctx).Save(&skill).Error
}

// Delete removes a user skill. System skills are read-only.
func (s *SkillService) Delete(ctx context.Context, id string) error {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if skill.IsSystem {
		return ErrSystemReadOnly
	}
	return s.db.WithContext(ctx).Delete(&repository.Skill{}, "id = ?", id).Error
}

// getOrCreateSystemSkill find-or-creates a system skill by unique name, returning its id.
func (s *SkillService) getOrCreateSystemSkill(ctx context.Context, name, description, body string) (string, error) {
	var existing repository.Skill
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	now := time.Now().UTC()
	skill := repository.Skill{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Body:        body,
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if createErr := s.db.WithContext(ctx).Create(&skill).Error; createErr != nil {
		if !isDuplicateKeyError(createErr) {
			return "", createErr
		}
		// Lost a race — fetch the winner.
		if err := s.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; err != nil {
			return "", err
		}
		return existing.ID, nil
	}
	return skill.ID, nil
}

// Init idempotently seeds the system default skills from the embedded seeding data.
func (s *SkillService) Init(ctx context.Context) error {
	for _, def := range seeding.SystemSkills {
		if _, err := s.getOrCreateSystemSkill(ctx, def.Name, def.Description, def.Body); err != nil {
			return err
		}
	}
	return nil
}
