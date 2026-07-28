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
		InputSchema: struct {
			Path string `json:"path" description:"Absolute or relative path to the directory to list."`
		}{},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "sandbox not yet available: file operations require the Phase 5 sandbox", nil
		},
	})
}
