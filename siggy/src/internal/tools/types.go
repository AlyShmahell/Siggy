package tools

import (
	"context"
	"encoding/json"

	"siggy/src/internal/harness"
)

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Risk() harness.Risk
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

type Delegator interface {
	Delegate(ctx context.Context, agent, task string) (string, error)
}

func mustSchema(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func objectSchema(props map[string]any, required []string) json.RawMessage {
	return mustSchema(map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	})
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}
