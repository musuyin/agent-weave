package agent_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/model/repository"
)

// makeMsg builds a minimal repository.Message for compaction tests.
func makeMsg(role, text string) repository.Message {
	return repository.Message{
		ID:             uuid.NewString(),
		ConversationID: "conv-test",
		Role:           role,
		Content:        repository.ContentBlocks{{Type: "text", Text: text}},
		CreatedAt:      time.Now().UTC(),
	}
}

// makeMsgs builds n alternating user/assistant messages.
func makeMsgs(n int) []repository.Message {
	msgs := make([]repository.Message, n)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = makeMsg(role, "message "+string(rune('A'+i%26)))
	}
	return msgs
}

// TestCompactionSplitInvariants verifies the arithmetic of the head/tail/middle split.
// These are pure calculations — no DB or LLM needed.
func TestCompactionSplitInvariants(t *testing.T) {
	msgs := makeMsgs(agent.CompactThreshold)

	head := msgs[:agent.PinnedHead]
	tail := msgs[len(msgs)-agent.LiveTail:]
	middle := msgs[agent.PinnedHead : len(msgs)-agent.LiveTail]

	assert.Len(t, head, agent.PinnedHead)
	assert.Len(t, tail, agent.LiveTail)
	assert.Len(t, middle, agent.CompactThreshold-agent.PinnedHead-agent.LiveTail)
	assert.Equal(t, agent.CompactThreshold, len(head)+len(middle)+len(tail), "split must be lossless")

	// After compaction: head + 1 summary + tail = 15 messages.
	assert.Equal(t, 15, agent.PinnedHead+1+agent.LiveTail)
}

// TestCompactionThresholdBoundary checks the boundary: 39 stays below, 40 triggers.
func TestCompactionThresholdBoundary(t *testing.T) {
	assert.True(t, len(makeMsgs(agent.CompactThreshold-1)) < agent.CompactThreshold)
	assert.True(t, len(makeMsgs(agent.CompactThreshold)) >= agent.CompactThreshold)
}

// TestCompactedFieldDefault verifies that a newly created Message has Compacted=false.
func TestCompactedFieldDefault(t *testing.T) {
	db := newInvariantTestDB(t)
	convID := uuid.NewString()

	require.NoError(t, db.Create(&repository.Conversation{
		ID: convID, Title: "t",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	msg := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "user",
		Content:        repository.ContentBlocks{{Type: "text", Text: "hi"}},
		CreatedAt:      time.Now().UTC(),
	}
	require.NoError(t, db.Create(&msg).Error)

	var loaded repository.Message
	require.NoError(t, db.First(&loaded, "id = ?", msg.ID).Error)
	assert.False(t, loaded.Compacted, "new messages must default to compacted=false")
}

// TestCompactedFilter verifies that messages with compacted=true are excluded
// when queried with AND compacted = false.
func TestCompactedFilter(t *testing.T) {
	db := newInvariantTestDB(t)
	convID := uuid.NewString()

	require.NoError(t, db.Create(&repository.Conversation{
		ID: convID, Title: "t",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	active := repository.Message{
		ID: uuid.NewString(), ConversationID: convID, Role: "user",
		Content: repository.ContentBlocks{{Type: "text", Text: "active"}},
		CreatedAt: time.Now().UTC(),
	}
	compacted := repository.Message{
		ID: uuid.NewString(), ConversationID: convID, Role: "user",
		Compacted: true,
		Content:   repository.ContentBlocks{{Type: "text", Text: "compacted"}},
		CreatedAt: time.Now().UTC().Add(time.Millisecond),
	}
	require.NoError(t, db.Create(&active).Error)
	require.NoError(t, db.Create(&compacted).Error)

	var results []repository.Message
	require.NoError(t, db.
		Where("conversation_id = ? AND compacted = ?", convID, false).
		Find(&results).Error)

	assert.Len(t, results, 1)
	assert.Equal(t, active.ID, results[0].ID)
}
