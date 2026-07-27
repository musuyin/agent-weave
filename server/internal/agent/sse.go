package agent

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// EventType is the SSE event name (maps to the "event:" field in the SSE wire format).
type EventType string

const (
	EventAgentStart      EventType = "agent_start"
	EventBlockStart      EventType = "block_start"
	EventBlockDelta      EventType = "block_delta"
	EventBlockStop       EventType = "block_stop"
	EventMessageAppended EventType = "message_appended"
	EventApprovalReq     EventType = "approval_requested"
	EventThreadStatus    EventType = "thread_status"
	EventRoundDone       EventType = "round_done"
	EventQueueDrained    EventType = "queue_drained"
)

// SSEEvent is a single server-sent event pushed to the client.
type SSEEvent struct {
	Type EventType `json:"type"`
	Data any       `json:"data,omitempty"`
}

// BlockStartData is the payload for EventBlockStart.
type BlockStartData struct {
	BlockID   string `json:"block_id"`
	BlockType string `json:"block_type"` // "text" | "tool_use"
	Index     int64  `json:"index"`
}

// BlockDeltaData is the payload for EventBlockDelta.
type BlockDeltaData struct {
	BlockID string `json:"block_id"`
	Text    string `json:"text"`
	Index   int64  `json:"index"`
}

// BlockStopData is the payload for EventBlockStop.
type BlockStopData struct {
	BlockID string `json:"block_id"`
	Index   int64  `json:"index"`
}

// Hub manages the SSE channel for one active conversation.
// Buffer size 256. Push uses drain-then-push to guarantee round_done/queue_drained delivery.
type Hub struct {
	ch     chan SSEEvent
	mu     sync.Mutex
	closed bool
	log    *slog.Logger
}

func newHub(log *slog.Logger) *Hub {
	return &Hub{
		ch:  make(chan SSEEvent, 256),
		log: log,
	}
}

// NewHub creates a new Hub. Exposed for use in tests and external packages.
func NewHub(log *slog.Logger) *Hub { return newHub(log) }

// Push sends an event to the channel.
// If the buffer is full, it drains all pending events first (the invariant: signal events
// like round_done and queue_drained must never be blocked or dropped).
func (h *Hub) Push(event SSEEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if len(h.ch) == cap(h.ch) {
		h.log.Warn("SSE channel full, draining", "discarded", len(h.ch))
		for len(h.ch) > 0 {
			<-h.ch
		}
	}
	h.ch <- event
}

// Chan returns the read-only channel for the SSE handler to range over.
func (h *Hub) Chan() <-chan SSEEvent { return h.ch }

// Cap returns the channel buffer capacity (useful in tests).
func (h *Hub) Cap() int { return cap(h.ch) }

// Len returns the current number of buffered events (useful in tests).
func (h *Hub) Len() int { return len(h.ch) }

// Close closes the hub. Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.closed = true
		close(h.ch)
	}
}

// JSON serialises an SSEEvent's Data field for the wire format.
func (e SSEEvent) DataJSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// HubRegistry maps conversationID → active Hub.
type HubRegistry struct {
	mu   sync.RWMutex
	hubs map[string]*Hub
	log  *slog.Logger
}

func NewHubRegistry(log *slog.Logger) *HubRegistry {
	return &HubRegistry{
		hubs: make(map[string]*Hub),
		log:  log,
	}
}

// GetOrCreate returns the existing Hub or creates a new one for the conversation.
func (r *HubRegistry) GetOrCreate(conversationID string) *Hub {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hubs[conversationID]; ok {
		return h
	}
	h := newHub(r.log)
	r.hubs[conversationID] = h
	return h
}

// Get returns the Hub for a conversation, or nil if none exists.
func (r *HubRegistry) Get(conversationID string) *Hub {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hubs[conversationID]
}

// Delete removes and closes the Hub for a conversation.
func (r *HubRegistry) Delete(conversationID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hubs[conversationID]; ok {
		h.Close()
		delete(r.hubs, conversationID)
	}
}
