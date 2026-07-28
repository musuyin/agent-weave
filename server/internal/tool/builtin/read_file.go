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
		InputSchema: struct {
			Path string `json:"path" description:"Absolute or relative path to the file to read."`
		}{},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "sandbox not yet available: file operations require the Phase 5 sandbox", nil
		},
	})
}
