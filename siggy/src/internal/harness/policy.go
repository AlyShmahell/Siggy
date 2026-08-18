package harness

import "fmt"

type Mode string

const (
	ModeChat Mode = "chat"
	ModePlan Mode = "plan"
	ModeAct  Mode = "act"
)

type Risk string

const (
	RiskRead    Risk = "read"
	RiskWrite   Risk = "write"
	RiskShell   Risk = "shell"
	RiskNetwork Risk = "network"
)

func (m Mode) Allows(risk Risk) error {
	if m != ModePlan {
		return nil
	}
	switch risk {
	case RiskWrite, RiskShell, RiskNetwork:
		return fmt.Errorf("plan mode blocks %s tools; switch to act with /act", risk)
	default:
		return nil
	}
}

func ParseMode(s string) Mode {
	switch Mode(s) {
	case ModeChat, ModePlan, ModeAct:
		return Mode(s)
	default:
		return ModeAct
	}
}
