package llm

import (
	"context"
	"encoding/json"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	Parts      []Part
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

type Part struct {
	Type string // "text" | "image"
	Text string
	MIME string
	Data []byte
}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type Request struct {
	Messages []Message
	Tools    []ToolSpec
}

type Usage struct {
	Prompt     int
	Completion int
	Total      int
}

type Chunk struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	Err       error
	Done      bool
}

type Client interface {
	Stream(ctx context.Context, req Request) (<-chan Chunk, error)
}

func Collect(ch <-chan Chunk) (text string, calls []ToolCall, err error) {
	for c := range ch {
		if c.Err != nil {
			return text, calls, c.Err
		}
		text += c.Text
		if len(c.ToolCalls) > 0 {
			calls = append(calls, c.ToolCalls...)
		}
	}
	return text, calls, nil
}
