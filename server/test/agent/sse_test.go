package agent_test

import (
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/agent"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestHub_PushAndReceive verifies that pushed events are received in order.
func TestHub_PushAndReceive(t *testing.T) {
	h := agent.NewHub(testLog())

	h.Push(agent.SSEEvent{Type: agent.EventBlockStart})
	h.Push(agent.SSEEvent{Type: agent.EventBlockDelta})
	h.Push(agent.SSEEvent{Type: agent.EventBlockStop})

	assert.Equal(t, agent.EventBlockStart, (<-h.Chan()).Type)
	assert.Equal(t, agent.EventBlockDelta, (<-h.Chan()).Type)
	assert.Equal(t, agent.EventBlockStop, (<-h.Chan()).Type)
}

// TestHub_DrainOnFull verifies invariant E: when the buffer is full,
// Push drains it rather than blocking, so the new event always lands.
func TestHub_DrainOnFull(t *testing.T) {
	h := agent.NewHub(testLog())

	// Fill the buffer completely.
	for i := 0; i < h.Cap(); i++ {
		h.Push(agent.SSEEvent{Type: agent.EventBlockDelta})
	}
	assert.Equal(t, h.Cap(), h.Len(), "buffer should be full")

	// Push a signal event — must not block and must be the only event in the channel.
	h.Push(agent.SSEEvent{Type: agent.EventRoundDone})

	assert.Equal(t, 1, h.Len(), "channel should contain exactly the signal event after drain")
	assert.Equal(t, agent.EventRoundDone, (<-h.Chan()).Type)
}

// TestHub_QueueDrainedAlwaysDelivered verifies the round_done → queue_drained
// sequence lands even when the buffer was full before each push.
func TestHub_QueueDrainedAlwaysDelivered(t *testing.T) {
	h := agent.NewHub(testLog())

	// Saturate with noise.
	for i := 0; i < h.Cap(); i++ {
		h.Push(agent.SSEEvent{Type: agent.EventBlockDelta})
	}
	h.Push(agent.SSEEvent{Type: agent.EventRoundDone})
	h.Push(agent.SSEEvent{Type: agent.EventQueueDrained})

	// Drain everything that arrived and confirm the last two are the signal pair.
	var events []agent.EventType
	for h.Len() > 0 {
		events = append(events, (<-h.Chan()).Type)
	}
	require.GreaterOrEqual(t, len(events), 2)
	assert.Equal(t, agent.EventRoundDone, events[len(events)-2])
	assert.Equal(t, agent.EventQueueDrained, events[len(events)-1])
}

// TestHub_CloseIdempotent verifies that calling Close twice does not panic.
func TestHub_CloseIdempotent(t *testing.T) {
	h := agent.NewHub(testLog())
	assert.NotPanics(t, func() {
		h.Close()
		h.Close()
	})
}

// TestHub_PushAfterClose verifies that pushing to a closed hub is a no-op (no panic).
func TestHub_PushAfterClose(t *testing.T) {
	h := agent.NewHub(testLog())
	h.Close()
	assert.NotPanics(t, func() {
		h.Push(agent.SSEEvent{Type: agent.EventRoundDone})
	})
}

// TestHub_ConcurrentPush verifies Push is safe under concurrent access.
func TestHub_ConcurrentPush(t *testing.T) {
	h := agent.NewHub(testLog())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				h.Push(agent.SSEEvent{Type: agent.EventBlockDelta})
			}
		}()
	}
	wg.Wait()
	// No assertion on count — buffer may have been drained mid-flight.
	// The goal is zero data races (run with -race).
}

// TestHubRegistry_GetOrCreate returns a stable Hub for the same key.
func TestHubRegistry_GetOrCreate(t *testing.T) {
	r := agent.NewHubRegistry(testLog())
	h1 := r.GetOrCreate("conv-1")
	h2 := r.GetOrCreate("conv-1")
	assert.Same(t, h1, h2, "same conversation must return the same Hub")

	h3 := r.GetOrCreate("conv-2")
	assert.NotSame(t, h1, h3, "different conversations must get different Hubs")
}

// TestHubRegistry_Delete closes and removes the Hub.
func TestHubRegistry_Delete(t *testing.T) {
	r := agent.NewHubRegistry(testLog())
	r.GetOrCreate("conv-1")
	r.Delete("conv-1")
	assert.Nil(t, r.Get("conv-1"), "hub should be gone after Delete")
}

// TestHubRegistry_DeleteIdempotent verifies deleting a non-existent key is safe.
func TestHubRegistry_DeleteIdempotent(t *testing.T) {
	r := agent.NewHubRegistry(testLog())
	assert.NotPanics(t, func() { r.Delete("no-such-conv") })
}
