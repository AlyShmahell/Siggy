package tools

import (
	"bytes"
	"context"
	"encoding/json"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Risk() harness.Risk
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

type Visual interface {
	RunVisual(ctx context.Context, args json.RawMessage) (string, []llm.Part, error)
}

type Delegator interface {
	Delegate(ctx context.Context, agent, task string) (string, error)
}

type RiskForArgs interface {
	RiskFor(args json.RawMessage) harness.Risk
}

func EffectiveRisk(t Tool, args json.RawMessage) harness.Risk {
	if r, ok := t.(RiskForArgs); ok {
		return r.RiskFor(args)
	}
	return t.Risk()
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
	raw = mergeArgs(raw)
	err := json.Unmarshal(raw, &v)
	return v, err
}

func mergeArgs(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	if json.Valid(raw) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	merged := map[string]any{}
	found := false
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			break
		}
		found = true
		for k, v := range obj {
			merged[k] = v
		}
	}
	if !found {
		return raw
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return raw
	}
	return out
}
