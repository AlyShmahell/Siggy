package tui

import (
	"strings"
	"testing"
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
