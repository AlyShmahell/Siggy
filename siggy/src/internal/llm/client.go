package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPClient struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func NewHTTP(baseURL, apiKey, model string) *HTTPClient {
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *HTTPClient) Ping(ctx context.Context) string {
	if c == nil {
		return "…"
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return "err"
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "err"
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "…"
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode >= 300 {
		return "err"
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "err"
	}
	for _, m := range payload.Data {
		if m.ID == c.Model {
			return "ok"
		}
	}
	return "err"
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaiRequest struct {
	Model         string            `json:"model"`
	Stream        bool              `json:"stream"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiTool         `json:"tools,omitempty"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *oaiUsage `json:"usage"`
}

func (c *HTTPClient) Stream(ctx context.Context, req Request) (<-chan Chunk, error) {
	body := oaiRequest{Model: c.Model, Stream: true, StreamOptions: &oaiStreamOptions{IncludeUsage: true}}
	for _, m := range req.Messages {
		om := oaiMessage{Role: string(m.Role), ToolCallID: m.ToolCallID, Name: m.Name}
		if m.Content != "" {
			om.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			call := oaiToolCall{ID: tc.ID, Type: "function"}
			call.Function.Name = tc.Name
			call.Function.Arguments = string(tc.Args)
			om.ToolCalls = append(om.ToolCalls, call)
		}
		body.Messages = append(body.Messages, om)
	}
	for _, t := range req.Tools {
		ot := oaiTool{Type: "function"}
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.Parameters
		if len(ot.Function.Parameters) == 0 {
			ot.Function.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		body.Tools = append(body.Tools, ot)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, bytes.TrimSpace(b))
	}

	out := make(chan Chunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		c.readStream(resp.Body, out)
	}()
	return out, nil
}

func (c *HTTPClient) readStream(r io.Reader, out chan<- Chunk) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	acc := map[int]*ToolCall{}
	var order []int
	flush := func() {
		if len(order) == 0 {
			return
		}
		var calls []ToolCall
		for _, i := range order {
			tc := acc[i]
			if tc.Args == nil {
				tc.Args = json.RawMessage(`{}`)
			}
			calls = append(calls, *tc)
		}
		out <- Chunk{ToolCalls: calls}
		acc = map[int]*ToolCall{}
		order = nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			flush()
			out <- Chunk{Done: true}
			return
		}
		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			out <- Chunk{Err: err}
			return
		}
		if chunk.Error != nil {
			out <- Chunk{Err: fmt.Errorf("%s", chunk.Error.Message)}
			return
		}
		if chunk.Usage != nil {
			out <- Chunk{Usage: Usage{
				Prompt:     chunk.Usage.PromptTokens,
				Completion: chunk.Usage.CompletionTokens,
				Total:      chunk.Usage.TotalTokens,
			}}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			out <- Chunk{Text: delta.Content}
		}
		for i, tc := range delta.ToolCalls {
			idx := i
			if existing, ok := acc[idx]; ok {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					existing.Args = append(existing.Args, []byte(tc.Function.Arguments)...)
				}
				continue
			}
			call := &ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: json.RawMessage(tc.Function.Arguments)}
			acc[idx] = call
			order = append(order, idx)
		}
		if chunk.Choices[0].FinishReason != "" {
			flush()
		}
	}
	if err := sc.Err(); err != nil {
		out <- Chunk{Err: err}
	}
}
