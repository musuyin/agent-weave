package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github/musuyin/agent-weave/internal/tool"
)

func init() {
	tool.Register(tool.ToolDef{
		Name:        "fetch_url",
		Description: "Fetch the content of a URL via HTTP GET and return it as a string. Result is truncated to 16 KB.",
		InputSchema: struct {
			URL string `json:"url" description:"The HTTP or HTTPS URL to fetch."`
		}{},
		Handler: fetchURLHandler,
	})
}

func fetchURLHandler(_ context.Context, params json.RawMessage) (string, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	u, err := url.Parse(input.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "error: url must be http or https", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return fmt.Sprintf("error: failed to build request: %v", err), nil
	}
	req.Header.Set("User-Agent", "agent-weave/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("error: request failed: %v", err), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Sprintf("HTTP error: %s", resp.Status), nil
	}

	// Read at most MaxToolResultBytes+1 to detect truncation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(tool.MaxToolResultBytes)+1))
	if err != nil {
		return fmt.Sprintf("error: reading response: %v", err), nil
	}

	return tool.Truncate(string(body), tool.MaxToolResultBytes), nil
}
