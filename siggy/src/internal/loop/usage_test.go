package loop

import (
	"testing"

	"siggy/src/internal/harness"
)

func TestBilledPrefersTotal(t *testing.T) {
	if got := Billed(639, 4983, 5622); got != 5622 {
		t.Fatalf("got %d", got)
	}
	if got := Billed(10, 5, 0); got != 15 {
		t.Fatalf("sum fallback = %d", got)
	}
}

func TestSumUsageAccumulates(t *testing.T) {
	recs := []harness.Record{
		{Type: "user", Text: "hi"},
		{Type: "usage", PromptTokens: 639, CompletionTokens: 4983, TotalTokens: 5622},
		{Type: "assistant", Text: "ok"},
		{Type: "usage", PromptTokens: 800, CompletionTokens: 50, TotalTokens: 850, Estimated: true},
	}
	prompt, billed, est := SumUsage(recs)
	if prompt != 800 {
		t.Fatalf("prompt = %d", prompt)
	}
	if billed != 6472 {
		t.Fatalf("billed = %d", billed)
	}
	if !est {
		t.Fatal("expected estimated")
	}
}
