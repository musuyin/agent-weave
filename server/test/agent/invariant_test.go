package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/tool"
	_ "github/musuyin/agent-weave/internal/tool/builtin"
)

func newInvariantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared&_loc=UTC"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&repository.Conversation{}, &repository.Message{}, &repository.Thread{}))
	return db
}

// dbAssertHook is a PreHook that records whether the assistant message exists in DB when it fires.
type dbAssertHook struct {
	db       *gorm.DB
	convID   string
	dbStates []bool // dbStates[i] = assistant row existed when tool i's FirePre fired
}

func (h *dbAssertHook) RunPre(_ context.Context, _ *hook.ToolCallParams) error {
	var count int64
	h.db.Model(&repository.Message{}).
		Where("conversation_id = ? AND role = ?", h.convID, "assistant").
		Count(&count)
	h.dbStates = append(h.dbStates, count > 0)
	return nil
}

// TestDispatchTools_InvariantA verifies that for a 2-tool response the assistant message
// is persisted in DB before FirePre fires for BOTH tools (not just the first).
func TestDispatchTools_InvariantA(t *testing.T) {
	db := newInvariantTestDB(t)
	convID := uuid.NewString()
	require.NoError(t, db.Create(&repository.Conversation{
		ID: convID, Title: "test", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	spy := &dbAssertHook{db: db, convID: convID}

	const dummyTool = "dummy_invariant_a"
	tool.Register(tool.ToolDef{
		Name:        dummyTool,
		Description: "test",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "ok", nil
		},
	})

	toolID1, toolID2 := uuid.NewString(), uuid.NewString()
	assistantBlocks := repository.ContentBlocks{
		{Type: "tool_use", ID: toolID1, Name: dummyTool, Input: json.RawMessage(`{}`)},
		{Type: "tool_use", ID: toolID2, Name: dummyTool, Input: json.RawMessage(`{}`)},
	}

	chain := hook.NewChain([]hook.PreHook{spy}, nil)

	require.NoError(t, agent.DispatchToolsForTest(context.Background(), db, convID, chain, assistantBlocks))

	require.Len(t, spy.dbStates, 2, "FirePre must be called once per tool")
	assert.True(t, spy.dbStates[0], "assistant message must be in DB before tool[0] FirePre")
	assert.True(t, spy.dbStates[1], "assistant message must be in DB before tool[1] FirePre")
}

// TestFetchURL_ContextCancellation verifies that a cancelled context aborts the HTTP request.
func TestFetchURL_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client cancels.
		<-r.Context().Done()
	}))
	defer srv.Close()

	def, ok := tool.Get("fetch_url")
	require.True(t, ok, "fetch_url must be registered via builtin import")

	ctx, cancel := context.WithCancel(context.Background())
	params, _ := json.Marshal(map[string]string{"url": srv.URL})

	resultCh := make(chan struct{}, 1)
	go func() {
		def.Handler(ctx, params) //nolint:errcheck
		resultCh <- struct{}{}
	}()

	cancel()

	select {
	case <-resultCh:
		// returned promptly — pass
	case <-time.After(3 * time.Second):
		t.Fatal("fetch_url did not respect context cancellation within 3 seconds")
	}
}
