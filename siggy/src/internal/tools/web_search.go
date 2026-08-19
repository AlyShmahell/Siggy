package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools/utils"
)

const ddgLite = "https://lite.duckduckgo.com/lite/"

type searchTool struct {
	h        *harness.Harness
	client   *http.Client
	endpoint string
}

func NewSearch(h *harness.Harness) Tool {
	return &searchTool{
		h:        h,
		client:   &http.Client{Timeout: 20 * time.Second},
		endpoint: ddgLite,
	}
}

func (t *searchTool) Name() string { return "web_search" }
func (t *searchTool) Description() string {
	return "Search the public web. Returns stripped search-page text (untrusted). Use web_fetch to read a result URL."
}
func (t *searchTool) Risk() harness.Risk { return harness.RiskNetwork }
func (t *searchTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"query": map[string]any{"type": "string"},
		"html":  map[string]any{"type": "boolean", "description": "If true, also return capped raw HTML"},
	}, []string{"query"})
}

func (t *searchTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Query string `json:"query"`
		HTML  bool   `json:"html"`
	}](raw)
	if err != nil {
		return "", err
	}
	q := strings.TrimSpace(args.Query)
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	endpoint := t.endpoint
	if endpoint == "" {
		endpoint = ddgLite
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	vals := u.Query()
	vals.Set("q", q)
	u.RawQuery = vals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "siggy/0.1")
	client := t.client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, utils.FetchRawCap))
	if err != nil {
		return "", err
	}
	md := utils.CapTextMarked(utils.HTMLToText(string(body), u.String()), utils.TextBodyCap)
	if t.h != nil {
		utils.WritePageCache(t.h.Home, "search", u.String(), body, md)
	}
	out := fmt.Sprintf("status %d\n%s", resp.StatusCode, md)
	if args.HTML {
		out += "\n" + utils.CapTextMarked(string(body), utils.TextBodyCap)
	}
	return out, nil
}
