package tools

import (
	"fmt"
	"sort"
	"sync"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) List() []Tool {
	names := r.Names()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n])
	}
	return out
}

func (r *Registry) Filter(allow []string) *Registry {
	if allow == nil {
		return r
	}
	set := map[string]bool{}
	for _, n := range allow {
		set[n] = true
	}
	next := NewRegistry()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, t := range r.tools {
		if set[name] {
			next.tools[name] = t
		}
	}
	return next
}

func (r *Registry) Specs() []llm.ToolSpec {
	var specs []llm.ToolSpec
	for _, t := range r.List() {
		specs = append(specs, llm.ToolSpec{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return specs
}

func Builtins(h *harness.Harness, d Delegator) *Registry {
	r := NewRegistry()
	r.Register(NewRead(h))
	r.Register(NewReadPDF(h))
	r.Register(NewWrite(h))
	r.Register(NewEdit(h))
	r.Register(NewList(h))
	r.Register(NewGlob(h))
	r.Register(NewGrep(h))
	r.Register(NewShell(h))
	r.Register(NewTodo(h))
	r.Register(NewFetch())
	r.Register(NewRemember(h))
	r.Register(NewForget(h))
	r.Register(NewSearchMemory(h))
	if d != nil {
		r.Register(NewDelegate(d))
	}
	return r
}

func Missing(name string) error {
	return fmt.Errorf("unknown tool %q", name)
}
