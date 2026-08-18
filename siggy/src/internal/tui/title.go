package tui

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

const untitledSession = "new agent"

type titleMsg struct {
	id    string
	title string
}

func sessionTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return untitledSession
	}
	return title
}

func (m *model) sessionLabel(id string) string {
	if m.sessionTitles != nil {
		if t, ok := m.sessionTitles[id]; ok && t != "" {
			return t
		}
	}
	if m.h != nil && m.h.Session != nil && m.h.Session.ID == id {
		return sessionTitle(m.h.Session.Meta.Title)
	}
	return untitledSession
}

func (m model) maybeTitle() tea.Cmd {
	if m.h == nil || m.h.Session == nil || m.g == nil || m.g.Engine == nil || m.g.Engine.LLM == nil {
		return nil
	}
	if strings.TrimSpace(m.h.Session.Meta.Title) != "" {
		return nil
	}
	user, asst := firstTurn(m.h.Session.Records())
	if user == "" || asst == "" {
		return nil
	}
	client := m.g.Engine.LLM
	sess := m.h.Session
	id := sess.ID
	return func() tea.Msg {
		title := requestSessionTitle(client, user, asst)
		if title == "" {
			title = fallbackTitle(user)
		}
		_ = sess.SetTitle(title)
		return titleMsg{id: id, title: title}
	}
}

func firstTurn(recs []harness.Record) (user, asst string) {
	for _, r := range recs {
		if user == "" && r.Type == "user" && strings.TrimSpace(r.Text) != "" {
			user = r.Text
		}
		if r.Type == "assistant" && strings.TrimSpace(r.Text) != "" {
			asst = r.Text
		}
	}
	return user, asst
}

func requestSessionTitle(client llm.Client, user, asst string) string {
	if client == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ch, err := client.Stream(ctx, llm.Request{Messages: []llm.Message{
		{Role: llm.RoleSystem, Content: "Reply with a 3-8 word conversation title only. No quotes or punctuation besides spaces."},
		{Role: llm.RoleUser, Content: "User: " + clipText(user, 400) + "\nAssistant: " + clipText(asst, 400)},
	}})
	if err != nil {
		return ""
	}
	text, _, err := llm.Collect(ch)
	if err != nil {
		return ""
	}
	return cleanTitle(text)
}

func fallbackTitle(user string) string {
	return cleanTitle(user)
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”`)
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) > 40 {
		r := []rune(s)
		s = string(r[:40])
	}
	return s
}

func clipText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
