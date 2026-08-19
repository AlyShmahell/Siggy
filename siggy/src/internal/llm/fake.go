package llm

import "context"

type Scripted struct {
	Steps []ScriptedStep
	i     int
}

type ScriptedStep struct {
	Text  string
	Calls []ToolCall
	Usage Usage
	Err   error
}

func (s *Scripted) Ping(_ context.Context) string { return "ok" }

func (s *Scripted) Stream(_ context.Context, _ Request) (<-chan Chunk, error) {
	ch := make(chan Chunk, 8)
	go func() {
		defer close(ch)
		if s.i >= len(s.Steps) {
			ch <- Chunk{Text: "done", Done: true}
			return
		}
		step := s.Steps[s.i]
		s.i++
		if step.Err != nil {
			ch <- Chunk{Err: step.Err}
			return
		}
		if step.Text != "" {
			ch <- Chunk{Text: step.Text}
		}
		if len(step.Calls) > 0 {
			ch <- Chunk{ToolCalls: step.Calls}
		}
		if step.Usage.Prompt > 0 || step.Usage.Completion > 0 || step.Usage.Total > 0 {
			ch <- Chunk{Usage: step.Usage}
		}
		ch <- Chunk{Done: true}
	}()
	return ch, nil
}
