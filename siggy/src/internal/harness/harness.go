package harness

type Harness struct {
	Workspace *Workspace
	Approvals *ApprovalBus
	Session   *Session
	Mode      Mode
	Loops     *LoopDetect
	Home      string
	Depth     int
}

func New(workspace, home string, autoApprove bool) (*Harness, error) {
	ws, err := NewWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	sess, err := NewSessionMeta(home, SessionMeta{
		CWD:           ws.Root,
		WorkspaceHash: HashWorkspace(ws.Root),
	})
	if err != nil {
		return nil, err
	}
	return &Harness{
		Workspace: ws,
		Approvals: NewApprovalBus(autoApprove),
		Session:   sess,
		Mode:      ModeAct,
		Loops:     NewLoopDetect(3),
		Home:      home,
	}, nil
}

func (h *Harness) Child() *Harness {
	cp := *h
	cp.Depth = h.Depth + 1
	cp.Loops = NewLoopDetect(3)
	return &cp
}
