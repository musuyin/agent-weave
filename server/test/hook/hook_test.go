package hook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/hook"
)

// --- TestFirePre_Abort ---

type failHook struct{ called bool }

func (h *failHook) RunPre(_ context.Context, _ *hook.ToolCallParams) error {
	h.called = true
	return &hook.ErrToolDenied{Name: "test-tool"}
}

type recordHook struct{ called bool }

func (h *recordHook) RunPre(_ context.Context, _ *hook.ToolCallParams) error {
	h.called = true
	return nil
}

func TestFirePre_Abort(t *testing.T) {
	first := &failHook{}
	second := &recordHook{}

	chain := hook.NewChain([]hook.PreHook{first, second}, nil)
	params := hook.ToolCallParams{Name: "test-tool", Params: json.RawMessage(`{}`)}

	err := chain.FirePre(context.Background(), &params)
	require.Error(t, err)
	assert.True(t, first.called, "first hook must have been called")
	assert.False(t, second.called, "second hook must not be called after first aborted")
}

// --- TestAuditHook_KeysOnly ---

func TestAuditHook_KeysOnly(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	h := hook.NewAuditHook(log)
	params := hook.ToolCallParams{
		Name:   "fetch_url",
		Params: json.RawMessage(`{"url":"https://secret.internal/token?key=abc123"}`),
	}

	// RunPost is called in a goroutine in the real chain, but the method itself is synchronous.
	h.RunPost(context.Background(), params, "ok", nil)

	output := buf.String()
	assert.Contains(t, output, "url", "param key 'url' must appear in log")
	assert.NotContains(t, output, "secret.internal", "param value must not appear in log")
	assert.NotContains(t, output, "abc123", "param value must not appear in log")
}

func TestAuditHook_LogsFailure(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	h := hook.NewAuditHook(log)
	params := hook.ToolCallParams{Name: "fetch_url", Params: json.RawMessage(`{"url":"x"}`)}
	h.RunPost(context.Background(), params, "", &hook.ErrToolDenied{Name: "fetch_url"})

	output := buf.String()
	assert.True(t, strings.Contains(output, "failed") || strings.Contains(output, "error"),
		"failure log must mention failure or error")
}
