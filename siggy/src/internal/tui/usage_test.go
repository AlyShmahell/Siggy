package tui

import (
	"strings"
	"testing"

	"siggy/src/internal/loop"
)

func TestUsageBadge(t *testing.T) {
	if !strings.Contains(usageBadge(0, 128000), "0%") {
		t.Fatalf("zero = %q", usageBadge(0, 128000))
	}
	if !strings.Contains(usageBadge(64000, 128000), "50%") {
		t.Fatalf("half = %q", usageBadge(64000, 128000))
	}
	if usageBadge(10, 0) != " — " {
		t.Fatalf("unknown = %q", usageBadge(10, 0))
	}
	if usageColor(0, 128000) != colAdd {
		t.Fatalf("0%% color = %s want %s", usageColor(0, 128000), colAdd)
	}
	if usageColor(12800, 128000) != colAdd {
		t.Fatalf("10%% color = %s want %s", usageColor(12800, 128000), colAdd)
	}
	if !hexGreenFamily(usageColor(0, 128000)) {
		t.Fatalf("0%% color not green: %s", usageColor(0, 128000))
	}
	if !hexGreenFamily(usageColor(12800, 128000)) {
		t.Fatalf("10%% color not green: %s", usageColor(12800, 128000))
	}
	if !hexRedFamily(usageColor(121600, 128000)) {
		t.Fatalf("95%% color not red: %s", usageColor(121600, 128000))
	}
}

func TestIdleUsageIncludesSystemAndTools(t *testing.T) {
	m := testModel(t)
	if m.tokensUsed != 0 {
		t.Fatalf("tokensUsed = %d", m.tokensUsed)
	}
	if m.g == nil || m.g.Engine == nil || len(m.g.Engine.Messages) == 0 {
		t.Fatal("expected system message on a new session")
	}
	if m.usageUsed() < 1 {
		t.Fatalf("idle used = %d", m.usageUsed())
	}
	p := m.usageParts()
	if p.System < 1 {
		t.Fatalf("system = %d", p.System)
	}
	if p.Tools < 1 {
		t.Fatalf("tools = %d", p.Tools)
	}
	br := usageBreakdownLine(p, 0)
	if !strings.Contains(br, "s") || !strings.Contains(br, "t") {
		t.Fatalf("breakdown = %q", br)
	}
	idle := m.usageUsed()
	m.ta.SetValue(strings.Repeat("hello ", 200))
	if m.usageParts().Draft < 1 {
		t.Fatalf("draft = %d", m.usageParts().Draft)
	}
	if m.usageUsed() <= idle {
		t.Fatalf("composer draft should raise estimate: idle=%d now=%d", idle, m.usageUsed())
	}
	next, _ := m.onEvent(loop.Event{Kind: loop.KindUsage, PromptTokens: 6400})
	nm := next.(model)
	if nm.usageUsed() != 6400 {
		t.Fatalf("usage event = %d", nm.usageUsed())
	}
	nm.float = floatUsage
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, "sys ") || !strings.Contains(plain, "tools ") {
		t.Fatalf("popup missing breakdown:\n%s", plain)
	}
}

func TestFormatTok(t *testing.T) {
	if formatTok(847) != "847" {
		t.Fatalf("847 = %q", formatTok(847))
	}
	if formatTok(1200) != "1.2k" {
		t.Fatalf("1200 = %q", formatTok(1200))
	}
	if formatTok(12000) != "12k" {
		t.Fatalf("12000 = %q", formatTok(12000))
	}
}
