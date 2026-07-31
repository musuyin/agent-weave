package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/agent"
)

func TestDispatchRegistry_AddPendingDone(t *testing.T) {
	reg := agent.NewDispatchRegistry()

	const convID = "conv-1"
	require.Equal(t, 0, reg.Pending(convID))

	reg.Add(convID)
	reg.Add(convID)
	assert.Equal(t, 2, reg.Pending(convID))

	reg.Done(convID, agent.SubAgentResult{AgentName: "a", ThreadID: "t1", Output: "out1"})
	assert.Equal(t, 1, reg.Pending(convID))

	reg.Done(convID, agent.SubAgentResult{AgentName: "b", ThreadID: "t2", Output: "out2"})
	assert.Equal(t, 0, reg.Pending(convID))
}

func TestDispatchRegistry_WaitAndDrain(t *testing.T) {
	reg := agent.NewDispatchRegistry()
	const convID = "conv-2"

	reg.Add(convID)
	go func() {
		reg.Done(convID, agent.SubAgentResult{AgentName: "x", ThreadID: "t3", Output: "hello"})
	}()

	reg.Wait(convID)

	results := reg.Drain(convID)
	require.Len(t, results, 1)
	assert.Equal(t, "x", results[0].AgentName)
	assert.Equal(t, "hello", results[0].Output)

	// After drain, pending and results are cleared.
	assert.Equal(t, 0, reg.Pending(convID))
	assert.Empty(t, reg.Drain(convID))
}

func TestDispatchRegistry_MultiConversation(t *testing.T) {
	reg := agent.NewDispatchRegistry()

	reg.Add("c1")
	reg.Add("c2")
	reg.Add("c2")

	assert.Equal(t, 1, reg.Pending("c1"))
	assert.Equal(t, 2, reg.Pending("c2"))

	reg.Done("c1", agent.SubAgentResult{AgentName: "a"})
	reg.Done("c2", agent.SubAgentResult{AgentName: "b"})

	assert.Equal(t, 0, reg.Pending("c1"))
	assert.Equal(t, 1, reg.Pending("c2"))
}
