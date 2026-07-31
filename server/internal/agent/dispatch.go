package agent

import "sync"

// SubAgentResult holds the output of one completed subagent run.
type SubAgentResult struct {
	AgentName string
	ThreadID  string
	Output    string // last text block emitted by the subagent's LLM response
	Err       error
}

// DispatchRegistry tracks in-flight subagent goroutines per conversation and
// collects their results for fan-in. One instance per application.
type DispatchRegistry struct {
	mu      sync.Mutex
	pending map[string]int             // convID → active goroutine count
	wgs     map[string]*sync.WaitGroup // convID → WaitGroup
	outs    map[string][]SubAgentResult
}

func NewDispatchRegistry() *DispatchRegistry {
	return &DispatchRegistry{
		pending: make(map[string]int),
		wgs:     make(map[string]*sync.WaitGroup),
		outs:    make(map[string][]SubAgentResult),
	}
}

// Add increments the pending count for convID before launching a subagent goroutine.
func (r *DispatchRegistry) Add(convID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.wgs[convID]; !ok {
		var wg sync.WaitGroup
		r.wgs[convID] = &wg
	}
	r.wgs[convID].Add(1)
	r.pending[convID]++
}

// Done records a subagent result and decrements the pending count.
func (r *DispatchRegistry) Done(convID string, result SubAgentResult) {
	r.mu.Lock()
	r.outs[convID] = append(r.outs[convID], result)
	wg := r.wgs[convID]
	r.pending[convID]--
	r.mu.Unlock()
	if wg != nil {
		wg.Done()
	}
}

// Pending returns the number of still-running subagent goroutines for convID.
func (r *DispatchRegistry) Pending(convID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending[convID]
}

// Wait blocks until all subagent goroutines for convID have called Done.
func (r *DispatchRegistry) Wait(convID string) {
	r.mu.Lock()
	wg := r.wgs[convID]
	r.mu.Unlock()
	if wg != nil {
		wg.Wait()
	}
}

// Drain returns and clears the accumulated results for convID.
func (r *DispatchRegistry) Drain(convID string) []SubAgentResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	results := r.outs[convID]
	delete(r.outs, convID)
	delete(r.wgs, convID)
	delete(r.pending, convID)
	return results
}
