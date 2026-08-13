package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github/musuyin/agent-weave/internal/tool"
)

// convIDFunc is the type for retrieving a conversation ID from a context.
type convIDFunc = func(context.Context) (string, bool)

// Manager creates and owns one Docker container per conversation.
type Manager struct {
	docker     *client.Client
	image      string
	baseDir    string // absolute host path for workspace mounts
	mu         sync.Mutex
	containers map[string]*Container
	log        *slog.Logger
}

// Ensure returns the existing container for convID, or creates and starts a new one.
func (m *Manager) Ensure(ctx context.Context, convID string) (*Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[convID]; ok {
		return c, nil
	}

	hostWorkspace := filepath.Join(m.baseDir, convID)
	if err := os.MkdirAll(hostWorkspace, 0o755); err != nil {
		return nil, fmt.Errorf("sandbox mkdir: %w", err)
	}

	resp, err := m.docker.ContainerCreate(ctx,
		&dockercontainer.Config{
			Image:      m.image,
			Cmd:        []string{"sleep", "infinity"},
			WorkingDir: "/workspace",
		},
		&dockercontainer.HostConfig{
			Binds:      []string{hostWorkspace + ":/workspace"},
			AutoRemove: true,
			Resources: dockercontainer.Resources{
				Memory: 256 << 20, // 256 MB
			},
		},
		nil, nil,
		"sandbox-"+convID[:8],
	)
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}

	if err := m.docker.ContainerStart(ctx, resp.ID, dockercontainer.StartOptions{}); err != nil {
		return nil, fmt.Errorf("container start: %w", err)
	}

	m.log.Info("sandbox container started", "conv_id", convID, "container_id", resp.ID[:12])

	c := &Container{id: resp.ID, mgr: m}
	m.containers[convID] = c
	return c, nil
}

// RegisterTools registers read_file, list_directory, write_file, and run_command
// with the global tool registry. Overwrites any stubs previously registered via init().
// convIDFromCtx retrieves the conversation ID injected by the agent loop.
func (m *Manager) RegisterTools(convIDFromCtx func(context.Context) (string, bool)) {
	tool.Register(tool.ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file in the conversation sandbox. Path is relative to /workspace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to /workspace."},
			},
			"required": []string{"path"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var p struct{ Path string `json:"path"` }
			if err := json.Unmarshal(raw, &p); err != nil {
				return "error: bad params", nil
			}
			convID, ok := convIDFromCtx(ctx)
			if !ok {
				return "error: no conversation context", nil
			}
			c, err := m.Ensure(ctx, convID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			return c.ReadFile(ctx, p.Path)
		},
	})

	tool.Register(tool.ToolDef{
		Name:        "list_directory",
		Description: "List the contents of a directory in the conversation sandbox. Path is relative to /workspace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Directory path relative to /workspace. Defaults to root workspace if empty."},
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var p struct{ Path string `json:"path"` }
			_ = json.Unmarshal(raw, &p)
			convID, ok := convIDFromCtx(ctx)
			if !ok {
				return "error: no conversation context", nil
			}
			c, err := m.Ensure(ctx, convID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			return c.ListDir(ctx, p.Path)
		},
	})

	tool.Register(tool.ToolDef{
		Name:        "write_file",
		Description: "Write content to a file in the conversation sandbox, creating parent directories as needed. Path is relative to /workspace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "File path relative to /workspace."},
				"content": map[string]any{"type": "string", "description": "Content to write."},
			},
			"required": []string{"path", "content"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var p struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return "error: bad params", nil
			}
			convID, ok := convIDFromCtx(ctx)
			if !ok {
				return "error: no conversation context", nil
			}
			c, err := m.Ensure(ctx, convID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			if err := c.WriteFile(ctx, p.Path, p.Content); err != nil {
				return "error: " + err.Error(), nil
			}
			return "ok: wrote " + p.Path, nil
		},
	})

	tool.Register(tool.ToolDef{
		Name:        "run_command",
		Description: "Execute a shell command inside the conversation sandbox. Runs in /workspace. Returns combined stdout and stderr, truncated to 16 KB.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute (passed to sh -c)."},
			},
			"required": []string{"command"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var p struct{ Command string `json:"command"` }
			if err := json.Unmarshal(raw, &p); err != nil {
				return "error: bad params", nil
			}
			convID, ok := convIDFromCtx(ctx)
			if !ok {
				return "error: no conversation context", nil
			}
			c, err := m.Ensure(ctx, convID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			return c.Exec(ctx, p.Command)
		},
	})
}

// Close stops all running sandbox containers.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for convID, c := range m.containers {
		if err := m.docker.ContainerStop(context.Background(), c.id, dockercontainer.StopOptions{}); err != nil {
			m.log.Warn("stop sandbox container", "conv_id", convID, "error", err)
		}
		delete(m.containers, convID)
	}
	return m.docker.Close()
}

// pullImage pulls the image if it is not already present. Non-fatal: logs on error.
func (m *Manager) pullImage(ctx context.Context) {
	rc, err := m.docker.ImagePull(ctx, m.image, dockerimage.PullOptions{})
	if err != nil {
		m.log.Warn("sandbox image pull failed (may already be present)", "image", m.image, "error", err)
		return
	}
	defer rc.Close()
	// Drain the pull response so the connection is properly closed.
	_, _ = io.Copy(io.Discard, rc)
	m.log.Info("sandbox image ready", "image", m.image)
}
