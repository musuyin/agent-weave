package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type reportDef struct {
	title         string
	buildPrompt   func(now time.Time) string
}

var reportDefs = map[string]reportDef{
	"daily": {
		title: "daily-report",
		buildPrompt: func(now time.Time) string {
			return fmt.Sprintf(
				`Generate a daily work report for %s (today).`+
					` Use the GitHub MCP tools to find:`+
					` commits pushed on %s and pull requests merged on %s.`+
					` Filter out bot authors whose login contains "serviceuser" or "[bot]".`+
					` Group results by repository and format as a Markdown summary.`,
				now.Format("2006-01-02"), now.Format("2006-01-02"), now.Format("2006-01-02"),
			)
		},
	},
	"weekly": {
		title: "weekly-report",
		buildPrompt: func(now time.Time) string {
			// Monday of the current week
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7 // Sunday → 7 so Monday offset is correct
			}
			monday := now.AddDate(0, 0, -(weekday - 1))
			return fmt.Sprintf(
				`Generate a weekly work report for the week of %s to %s (Monday to today).`+
					` Use the GitHub MCP tools to find:`+
					` commits pushed from %s to %s and pull requests opened and merged during this period.`+
					` Filter out bot authors whose login contains "serviceuser" or "[bot]".`+
					` Group results by repository and format as a copyable plain-text bullet list.`,
				monday.Format("2006-01-02"), now.Format("2006-01-02"),
				monday.Format("2006-01-02"), now.Format("2006-01-02"),
			)
		},
	},
}

type ReportService struct {
	db      *gorm.DB
	convSvc *ConversationService
}

func NewReportService(db *gorm.DB, convSvc *ConversationService) *ReportService {
	return &ReportService{db: db, convSvc: convSvc}
}

// ProvideReportService creates the ReportService and pre-creates all report conversations at startup.
func ProvideReportService(ctx context.Context, db *gorm.DB, convSvc *ConversationService) (*ReportService, error) {
	svc := NewReportService(db, convSvc)
	if err := svc.Init(ctx); err != nil {
		return nil, err
	}
	return svc, nil
}

// ReportDef returns the title and a date-stamped prompt for the given report type, or an error for unknown types.
func (s *ReportService) ReportDef(reportType string) (title, prompt string, err error) {
	def, ok := reportDefs[reportType]
	if !ok {
		return "", "", fmt.Errorf("unknown report type: %s", reportType)
	}
	return def.title, def.buildPrompt(time.Now()), nil
}

// Init idempotently pre-creates all report conversations so their IDs are stable at startup.
func (s *ReportService) Init(ctx context.Context) error {
	for _, def := range reportDefs {
		if _, err := s.GetOrCreateReportConversation(ctx, def.title); err != nil {
			return fmt.Errorf("init report conversation %q: %w", def.title, err)
		}
	}
	return nil
}

// GetOrCreateReportConversation returns the ID of the conversation with the given title,
// creating it if absent. Safe against concurrent creates: on duplicate key the winner's row is returned.
func (s *ReportService) GetOrCreateReportConversation(ctx context.Context, title string) (string, error) {
	conv, err := s.convSvc.FindByTitle(ctx, title)
	if err == nil {
		return conv.ID, nil
	}

	created, createErr := s.convSvc.Create(ctx, title)
	if createErr == nil {
		return created.ID, nil
	}
	if !isDuplicateKeyError(createErr) {
		return "", createErr
	}

	// Lost a concurrent create race — fetch the winner's row.
	conv, err = s.convSvc.FindByTitle(ctx, title)
	return conv.ID, err
}
