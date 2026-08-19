package loop

import (
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

func Billed(prompt, completion, total int) int {
	if total > 0 {
		return total
	}
	return prompt + completion
}

func SumUsage(recs []harness.Record) (prompt, billed int, estimated bool) {
	for _, r := range recs {
		if r.Type != "usage" {
			continue
		}
		if r.PromptTokens > 0 {
			prompt = r.PromptTokens
		}
		billed += Billed(r.PromptTokens, r.CompletionTokens, r.TotalTokens)
		if r.Estimated {
			estimated = true
		}
	}
	return prompt, billed, estimated
}

func usageFromChunk(u llm.Usage) bool {
	return u.Prompt > 0 || u.Completion > 0 || u.Total > 0 || u.Reasoning > 0
}
