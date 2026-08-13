package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github/musuyin/agent-weave/internal/tool"
)

// Container wraps a running Docker container for one conversation.
type Container struct {
	id  string
	mgr *Manager
}

// ReadFile reads the file at path (relative to /workspace) from the container.
func (c *Container) ReadFile(ctx context.Context, path string) (string, error) {
	resolved, err := validatePath(path)
	if err != nil {
		return "error: " + err.Error(), nil
	}
	return c.exec(ctx, []string{"cat", resolved})
}

// ListDir lists the directory at path (relative to /workspace) inside the container.
func (c *Container) ListDir(ctx context.Context, path string) (string, error) {
	if path == "" {
		path = "."
	}
	resolved, err := validatePath(path)
	if err != nil {
		return "error: " + err.Error(), nil
	}
	return c.exec(ctx, []string{"ls", "-la", resolved})
}

// WriteFile writes content to path (relative to /workspace), creating parent dirs as needed.
// Uses CopyToContainer with an in-memory tar archive to avoid shell injection.
func (c *Container) WriteFile(ctx context.Context, path, content string) error {
	_, err := validatePath(path)
	if err != nil {
		return err
	}

	cleanPath := filepath.Join("/workspace", filepath.Clean(path))
	dir := filepath.Dir(cleanPath)
	filename := filepath.Base(cleanPath)

	// Ensure parent directory exists.
	if _, err := c.execRaw(ctx, []string{"mkdir", "-p", dir}); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:    filename,
		Size:    int64(len(content)),
		Mode:    0o644,
		ModTime: time.Now(),
	}); err != nil {
		return fmt.Errorf("tar header: %w", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		return fmt.Errorf("tar write: %w", err)
	}
	_ = tw.Close()

	return c.mgr.docker.CopyToContainer(ctx, c.id, dir, buf, dockercontainer.CopyToContainerOptions{})
}

// EditFile replaces the first occurrence of oldStr with newStr in the file at path.
// Reads the full file without truncation so large files are handled correctly.
func (c *Container) EditFile(ctx context.Context, path, oldStr, newStr string) (string, error) {
	_, err := validatePath(path)
	if err != nil {
		return "error: " + err.Error(), nil
	}
	resolved := filepath.Join("/workspace", filepath.Clean(path))

	content, err := c.execRaw(ctx, []string{"cat", resolved})
	if err != nil {
		return "error reading file: " + err.Error(), nil
	}
	if !strings.Contains(content, oldStr) {
		return fmt.Sprintf("error: old_str not found in %s", path), nil
	}
	newContent := strings.Replace(content, oldStr, newStr, 1)
	if err := c.WriteFile(ctx, path, newContent); err != nil {
		return "error writing file: " + err.Error(), nil
	}
	return "ok: edited " + path, nil
}

// Exec runs command (passed to sh -c) inside the container and returns combined stdout+stderr.
func (c *Container) Exec(ctx context.Context, command string) (string, error) {
	return c.exec(ctx, []string{"sh", "-c", command})
}

// exec runs a command and returns output truncated to MaxToolResultBytes.
func (c *Container) exec(ctx context.Context, cmd []string) (string, error) {
	s, err := c.execRaw(ctx, cmd)
	if err != nil {
		return "", err
	}
	return tool.Truncate(s, tool.MaxToolResultBytes), nil
}

// execRaw is the low-level Docker exec helper. Returns full combined output without truncation.
func (c *Container) execRaw(ctx context.Context, cmd []string) (string, error) {
	execResp, err := c.mgr.docker.ContainerExecCreate(ctx, c.id, dockercontainer.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	attach, err := c.mgr.docker.ContainerExecAttach(ctx, execResp.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()

	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, attach.Reader); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("exec read: %w", err)
	}

	return buf.String(), nil
}

// validatePath ensures path stays within /workspace after cleaning.
// Returns the absolute in-container path.
func validatePath(path string) (string, error) {
	resolved := filepath.Join("/workspace", filepath.Clean(path))
	if resolved != "/workspace" && !strings.HasPrefix(resolved, "/workspace/") {
		return "", errors.New("path escapes sandbox")
	}
	return resolved, nil
}
