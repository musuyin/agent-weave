package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github/musuyin/agent-weave/internal/tool"
)

// headerTransport injects static headers into every HTTP request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}

// HTTPClient connects to a single MCP server over HTTP streamable transport.
type HTTPClient struct {
	session *sdkmcp.ClientSession
}

// NewHTTPClient dials the MCP server at url, injects headers for auth, and completes the MCP handshake.
func NewHTTPClient(ctx context.Context, url string, headers map[string]string) (*HTTPClient, error) {
	httpClient := &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}
	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: httpClient,
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "agent-weave",
		Version: "0.1.0",
	}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect %s: %w", url, err)
	}
	return &HTTPClient{session: session}, nil
}

func (c *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			schema = json.RawMessage("{}")
		}
		tools = append(tools, Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return tools, nil
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, params json.RawMessage) (string, error) {
	var args map[string]any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &args); err != nil {
			return "", fmt.Errorf("unmarshal tool params: %w", err)
		}
	}
	result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, content := range result.Content {
		if tc, ok := content.(*sdkmcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	text := sb.String()
	return tool.Truncate(text, tool.MaxToolResultBytes), nil
}

func (c *HTTPClient) Close() error {
	c.session.Close()
	return nil
}
