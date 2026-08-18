package loop

import "siggy/src/internal/harness"

type Kind string

const (
	KindText      Kind = "text-delta"
	KindToolStart Kind = "tool-start"
	KindToolEnd   Kind = "tool-end"
	KindApproval  Kind = "approval-needed"
	KindError     Kind = "error"
	KindDone      Kind = "done"
	KindNode      Kind = "node"
	KindMode      Kind = "mode"
	KindSystem    Kind = "system"
	KindUsage     Kind = "usage"
)

type Event struct {
	Kind         Kind
	Text         string
	Tool         string
	CallID       string
	Args         string
	Err          error
	Node         string
	Mode         string
	Approval     *harness.ApprovalRequest
	PromptTokens int
	TotalTokens  int
}
