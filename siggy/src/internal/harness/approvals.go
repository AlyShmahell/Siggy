package harness

import (
	"context"
	"sync"
)

type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowSession
)

func (d Decision) Allowed() bool {
	return d == AllowOnce || d == AllowSession
}

type ApprovalRequest struct {
	ID      string
	Tool    string
	Summary string
	Risk    string
	Reply   chan Decision
}

type ApprovalBus struct {
	mu       sync.Mutex
	auto    bool
	session map[string]bool
	pending chan ApprovalRequest
}

func NewApprovalBus(auto bool) *ApprovalBus {
	return &ApprovalBus{
		auto:    auto,
		session: map[string]bool{},
		pending: make(chan ApprovalRequest, 16),
	}
}

func (b *ApprovalBus) SetAuto(auto bool) {
	b.mu.Lock()
	b.auto = auto
	b.mu.Unlock()
}

func (b *ApprovalBus) Auto() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.auto
}

func (b *ApprovalBus) AllowSession(tool string) {
	b.mu.Lock()
	b.session[tool] = true
	b.mu.Unlock()
}

func (b *ApprovalBus) Ask(ctx context.Context, req ApprovalRequest) (Decision, error) {
	b.mu.Lock()
	if b.auto || b.session[req.Tool] {
		b.mu.Unlock()
		return AllowSession, nil
	}
	b.mu.Unlock()

	if req.Reply == nil {
		req.Reply = make(chan Decision, 1)
	}
	select {
	case b.pending <- req:
	default:
	}
	select {
	case d := <-req.Reply:
		if d == AllowSession {
			b.AllowSession(req.Tool)
		}
		return d, nil
	case <-ctx.Done():
		return Deny, ctx.Err()
	}
}

func (b *ApprovalBus) Next(ctx context.Context) (ApprovalRequest, error) {
	select {
	case req := <-b.pending:
		return req, nil
	case <-ctx.Done():
		return ApprovalRequest{}, ctx.Err()
	}
}

func (b *ApprovalBus) TryNext() (ApprovalRequest, bool) {
	select {
	case req := <-b.pending:
		return req, true
	default:
		return ApprovalRequest{}, false
	}
}
