package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

func highlightCode(lang, code string) string {
	if lang == "" {
		return ""
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		return ""
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, tok := range it.Tokens() {
		b.WriteString(chromaStyle(tok.Type).Render(tok.Value))
	}
	return b.String()
}

func chromaStyle(t chroma.TokenType) lipgloss.Style {
	st := lipgloss.NewStyle().Background(colPanel)
	switch {
	case t.InCategory(chroma.Keyword):
		return st.Foreground(colAccent)
	case t.InSubCategory(chroma.String):
		return st.Foreground(colOk)
	case t.InCategory(chroma.Comment):
		return st.Foreground(colMuted)
	case t.InCategory(chroma.Error):
		return st.Foreground(colErr)
	case t.InSubCategory(chroma.NameFunction):
		return st.Foreground(colAsst)
	case t.InSubCategory(chroma.Number):
		return st.Foreground(colTool)
	default:
		return st.Foreground(colFg)
	}
}
