package builtin

import (
	"context"
	"encoding/json"

	"github/musuyin/agent-weave/internal/tool"
)

func init() {
	tool.Register(tool.ToolDef{
		Name:        "list_directory",
		Description: "List the contents of a directory at the given path. Returns a JSON array of entry names.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the directory to list.",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "sandbox not yet available: file operations require the Phase 5 sandbox", nil
		},
	})
}
