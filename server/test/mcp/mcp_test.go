package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalmcp "github/musuyin/agent-weave/internal/mcp"
)

// ---- SchemaFields tests ----

func TestSchemaFields_MapInput(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string"},
		},
		"required": []any{"url"},
	}
	props, req := internalmcp.SchemaFields(schema)
	assert.NotNil(t, props)
	assert.Equal(t, []string{"url"}, req)
}

func TestSchemaFields_RawJSONInput(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "object",
		"properties": {"repo": {"type": "string"}},
		"required": ["repo", "owner"]
	}`)
	props, req := internalmcp.SchemaFields(raw)
	assert.NotNil(t, props)
	assert.Equal(t, []string{"repo", "owner"}, req)
}

func TestSchemaFields_InvalidInput(t *testing.T) {
	props, req := internalmcp.SchemaFields(42)
	assert.Nil(t, props)
	assert.Nil(t, req)
}

func TestSchemaFields_NoRequired(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "string"}},
	}
	props, req := internalmcp.SchemaFields(schema)
	assert.NotNil(t, props)
	assert.Nil(t, req)
}

// ---- Router tests ----

// fakeClient is a no-op Client for testing the router without a real MCP server.
type fakeClient struct {
	tools []internalmcp.Tool
}

func (f *fakeClient) ListTools(_ context.Context) ([]internalmcp.Tool, error) {
	return f.tools, nil
}
func (f *fakeClient) CallTool(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	return "ok", nil
}
func (f *fakeClient) Close() error { return nil }

func newTestRouter(prefix string, toolNames []string) *internalmcp.Router {
	tools := make([]internalmcp.Tool, 0, len(toolNames))
	for _, name := range toolNames {
		tools = append(tools, internalmcp.Tool{
			Name:        name,
			Description: "desc:" + name,
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		})
	}
	client := &fakeClient{tools: tools}
	return internalmcp.NewRouterForTest(prefix, client, tools)
}

func TestRouter_Route(t *testing.T) {
	r := newTestRouter("gh", []string{"list_commits", "search_code"})

	client, unprefixed, ok := r.Route("gh__list_commits")
	require.True(t, ok)
	assert.NotNil(t, client)
	assert.Equal(t, "list_commits", unprefixed)

	_, _, ok = r.Route("gh__unknown")
	assert.False(t, ok)
}

func TestRouter_AllTools(t *testing.T) {
	r := newTestRouter("github-tools", []string{"list_commits", "search_issues"})

	defs := r.AllTools()
	require.Len(t, defs, 2)

	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	assert.Contains(t, names, "github-tools__list_commits")
	assert.Contains(t, names, "github-tools__search_issues")

	for _, d := range defs {
		assert.Nil(t, d.Handler, "MCP ToolDef.Handler must be nil — dispatched via Route()")
	}
}

func TestRouter_Empty(t *testing.T) {
	r := internalmcp.NewRouterForTest("", nil, nil)

	_, _, ok := r.Route("anything")
	assert.False(t, ok)
	assert.Empty(t, r.AllTools())
}

func TestRouter_SchemaPreservedInAllTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"}},"required":["owner"]}`)
	tools := []internalmcp.Tool{{Name: "get_repo", Description: "get a repo", InputSchema: schema}}
	r := internalmcp.NewRouterForTest("gh", &fakeClient{tools: tools}, tools)

	defs := r.AllTools()
	require.Len(t, defs, 1)

	props, req := internalmcp.SchemaFields(defs[0].InputSchema)
	assert.NotNil(t, props)
	assert.Equal(t, []string{"owner"}, req)
}

func TestRouter_NoPrefixPassthrough(t *testing.T) {
	r := newTestRouter("", []string{"my_tool"})

	_, unprefixed, ok := r.Route("my_tool")
	require.True(t, ok)
	assert.Equal(t, "my_tool", unprefixed)
}

func TestRouter_CallTool_Roundtrip(t *testing.T) {
	r := newTestRouter("svc", []string{"echo"})
	client, unprefixed, ok := r.Route("svc__echo")
	require.True(t, ok)

	result, err := client.CallTool(context.Background(), unprefixed, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestRouter_ToolDefHandler_IsNil(t *testing.T) {
	r := newTestRouter("x", []string{"foo"})
	for _, d := range r.AllTools() {
		assert.Nil(t, d.Handler, "calling a nil Handler on MCP tools would panic")
	}
}

func TestRouterForTestWithNilTools(t *testing.T) {
	r := internalmcp.NewRouterForTest("pfx", nil, nil)
	assert.Empty(t, r.AllTools())
	_, _, ok := r.Route("pfx__anything")
	assert.False(t, ok)
}

// Compile-time check: *internalmcp.Router satisfies no specific interface, but
// this ensures the exported API compiles correctly under the test package.
var _ = (*internalmcp.Router)(nil)

// Verify ToolDef slice returns the right types for the schema helper.
func TestSchemaFields_ToolDefRoundtrip(t *testing.T) {
	r := newTestRouter("svc", []string{"my_tool"})
	defs := r.AllTools()
	require.Len(t, defs, 1)

	// InputSchema stored as json.RawMessage; SchemaFields must handle it.
	props, _ := internalmcp.SchemaFields(defs[0].InputSchema)
	// The test router uses `{"type":"object","properties":{}}` — properties is present but empty map.
	assert.NotNil(t, props)
}

// TestSchemaFields_PartialSchema: schema with no "required" key.
func TestSchemaFields_PartialSchema(t *testing.T) {
	raw := json.RawMessage(`{"type":"object"}`)
	props, req := internalmcp.SchemaFields(raw)
	assert.Nil(t, props) // no "properties" key
	assert.Nil(t, req)
}
