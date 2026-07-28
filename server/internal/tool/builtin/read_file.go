package builtin

import (
	"context"
	"encoding/json"

	"github/musuyin/agent-weave/internal/tool"
)

func init() {
	tool.Register(tool.ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path. Returns the file content as a string.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file to read.",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "sandbox not yet available: file operations require the Phase 5 sandbox", nil
		},
	})
}
