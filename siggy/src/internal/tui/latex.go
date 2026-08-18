package tui

import (
	"strings"
	"unicode/utf8"
)

var latexSym = map[string]string{
	`\alpha`:      "α",
	`\beta`:       "β",
	`\gamma`:      "γ",
	`\delta`:      "δ",
	`\epsilon`:    "ε",
	`\varepsilon`: "ε",
	`\theta`:      "θ",
	`\lambda`:     "λ",
	`\mu`:         "μ",
	`\pi`:         "π",
	`\sigma`:      "σ",
	`\omega`:      "ω",
	`\infty`:      "∞",
	`\sum`:        "∑",
	`\prod`:       "∏",
	`\int`:        "∫",
	`\to`:         "→",
	`\rightarrow`: "→",
	`\leftarrow`:  "←",
	`\leq`:        "≤",
	`\geq`:        "≥",
	`\le`:         "≤",
	`\ge`:         "≥",
	`\lt`:         "<",
	`\gt`:         ">",
	`\neq`:        "≠",
	`\ne`:         "≠",
	`\approx`:     "≈",
	`\times`:      "×",
	`\cdot`:       "·",
	`\ast`:        "∗",
	`\pm`:         "±",
	`\cap`:        "∩",
	`\cup`:        "∪",
	`\in`:         "∈",
	`\notin`:      "∉",
	`\subset`:     "⊂",
	`\forall`:     "∀",
	`\exists`:     "∃",
	`\nabla`:      "∇",
	`\partial`:    "∂",
	`\sqrt`:       "√",
	`\dots`:       "…",
	`\ldots`:      "…",
	`\cdots`:      "⋯",
}

var latexPassthrough = map[string]bool{
	`\mathrm`:       true,
	`\mathbf`:       true,
	`\mathit`:       true,
	`\text`:         true,
	`\operatorname`: true,
}

var latexSuper = map[rune]rune{
	'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴',
	'5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹',
	'+': '⁺', '-': '⁻', '=': '⁼', '(': '⁽', ')': '⁾',
	'a': 'ᵃ', 'b': 'ᵇ', 'c': 'ᶜ', 'd': 'ᵈ', 'e': 'ᵉ',
	'f': 'ᶠ', 'g': 'ᵍ', 'h': 'ʰ', 'i': 'ⁱ', 'j': 'ʲ',
	'k': 'ᵏ', 'l': 'ˡ', 'm': 'ᵐ', 'n': 'ⁿ', 'o': 'ᵒ',
	'p': 'ᵖ', 'r': 'ʳ', 's': 'ˢ', 't': 'ᵗ', 'u': 'ᵘ',
	'v': 'ᵛ', 'w': 'ʷ', 'x': 'ˣ', 'y': 'ʸ', 'z': 'ᶻ',
	'A': 'ᴬ', 'B': 'ᴮ', 'D': 'ᴰ', 'E': 'ᴱ', 'G': 'ᴳ',
	'H': 'ᴴ', 'I': 'ᴵ', 'J': 'ᴶ', 'K': 'ᴷ', 'L': 'ᴸ',
	'M': 'ᴹ', 'N': 'ᴺ', 'O': 'ᴼ', 'P': 'ᴾ', 'R': 'ᴿ',
	'T': 'ᵀ', 'U': 'ᵁ', 'V': 'ⱽ', 'W': 'ᵂ',
}

var latexSub = map[rune]rune{
	'0': '₀', '1': '₁', '2': '₂', '3': '₃', '4': '₄',
	'5': '₅', '6': '₆', '7': '₇', '8': '₈', '9': '₉',
	'+': '₊', '-': '₋', '=': '₌', '(': '₍', ')': '₎',
	'a': 'ₐ', 'e': 'ₑ', 'h': 'ₕ', 'i': 'ᵢ', 'j': 'ⱼ',
	'k': 'ₖ', 'l': 'ₗ', 'm': 'ₘ', 'n': 'ₙ', 'o': 'ₒ',
	'p': 'ₚ', 'r': 'ᵣ', 's': 'ₛ', 't': 'ₜ', 'u': 'ᵤ',
	'v': 'ᵥ', 'x': 'ₓ',
}

func expandLatex(segs []mdSeg) []mdSeg {
	var out []mdSeg
	for _, s := range segs {
		if s.kind != mdProse {
			out = append(out, s)
			continue
		}
		for _, p := range splitLatex(s.text) {
			if p.kind == mdProse {
				p.text = expandBare(p.text)
			}
			out = append(out, p)
		}
	}
	return out
}

func expandBare(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	r := []rune(s)
	var b strings.Builder
	for i := 0; i < len(r); {
		if r[i] == '\\' {
			text, j := latexCommand(r, i)
			b.WriteString(text)
			i = j
			continue
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

func splitLatex(s string) []mdSeg {
	if s == "" {
		return nil
	}
	r := []rune(s)
	var out []mdSeg
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, mdSeg{kind: mdProse, text: cur.String()})
		cur.Reset()
	}
	emit := func(inner, kind string) {
		flush()
		out = append(out, mdSeg{kind: mdLatex, lang: kind, text: inner})
	}
	for i := 0; i < len(r); {
		if r[i] == '\\' && i+1 < len(r) {
			switch r[i+1] {
			case '\\':
				cur.WriteString(`\\`)
				i += 2
				continue
			case '$':
				cur.WriteRune('$')
				i += 2
				continue
			case '(':
				if inner, j, ok := closePair(r, i+2, ')', true); ok {
					if strings.TrimSpace(inner) == "" {
						cur.WriteString(string(r[i:j]))
					} else {
						emit(inner, "inline")
					}
					i = j
					continue
				}
			case '[':
				if inner, j, ok := closePair(r, i+2, ']', false); ok {
					if strings.TrimSpace(inner) == "" {
						cur.WriteString(string(r[i:j]))
					} else {
						emit(inner, "display")
					}
					i = j
					continue
				}
			}
		}
		if r[i] == '$' {
			display := i+1 < len(r) && r[i+1] == '$'
			start := i + 1
			if display {
				start = i + 2
			}
			if inner, j, ok := closeDollar(r, start, display); ok {
				if strings.TrimSpace(inner) == "" {
					cur.WriteString(string(r[i:j]))
				} else if display {
					emit(inner, "display")
				} else {
					emit(inner, "inline")
				}
				i = j
				continue
			}
		}
		cur.WriteRune(r[i])
		i++
	}
	flush()
	if len(out) == 0 {
		return []mdSeg{{kind: mdProse, text: s}}
	}
	return out
}

func closePair(r []rune, i int, closer rune, inline bool) (inner string, end int, ok bool) {
	j := i
	for j < len(r) {
		if inline && r[j] == '\n' {
			return "", 0, false
		}
		if r[j] == '\\' && j+1 < len(r) {
			if r[j+1] == closer {
				return string(r[i:j]), j + 2, true
			}
			j += 2
			continue
		}
		j++
	}
	return "", 0, false
}

func closeDollar(r []rune, i int, display bool) (inner string, end int, ok bool) {
	j := i
	for j < len(r) {
		if !display && r[j] == '\n' {
			return "", 0, false
		}
		if r[j] == '\\' && j+1 < len(r) {
			j += 2
			continue
		}
		if display {
			if r[j] == '$' && j+1 < len(r) && r[j+1] == '$' {
				return string(r[i:j]), j + 2, true
			}
		} else if r[j] == '$' {
			return string(r[i:j]), j + 1, true
		}
		j++
	}
	return "", 0, false
}

func renderLatex(s string) string {
	out, _ := latexRun([]rune(s), 0, false)
	return out
}

func latexRun(r []rune, i int, stopBrace bool) (string, int) {
	var b strings.Builder
	for i < len(r) {
		if stopBrace && r[i] == '}' {
			return b.String(), i
		}
		if r[i] == '{' {
			inner, j := latexRun(r, i+1, true)
			b.WriteString(inner)
			if j < len(r) && r[j] == '}' {
				j++
			}
			i = j
			continue
		}
		if r[i] == '^' || r[i] == '_' {
			script := r[i]
			atom, j := latexGroup(r, i+1)
			if script == '^' {
				b.WriteString(mapScript(atom, latexSuper, "^"))
			} else {
				b.WriteString(mapScript(atom, latexSub, "_"))
			}
			i = j
			continue
		}
		if r[i] == '\\' {
			text, j := latexCommand(r, i)
			b.WriteString(text)
			i = j
			continue
		}
		if r[i] == '}' {
			i++
			continue
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String(), i
}

func latexGroup(r []rune, i int) (string, int) {
	if i >= len(r) {
		return "", i
	}
	if r[i] == '{' {
		inner, j := latexRun(r, i+1, true)
		if j < len(r) && r[j] == '}' {
			j++
		}
		return inner, j
	}
	if r[i] == '\\' {
		return latexCommand(r, i)
	}
	return string(r[i]), i + 1
}

func latexCommand(r []rune, i int) (string, int) {
	if i >= len(r) || r[i] != '\\' {
		return "", i
	}
	j := i + 1
	if j < len(r) && !isLatexLetter(r[j]) {
		return string(r[j]), j + 1
	}
	for j < len(r) && isLatexLetter(r[j]) {
		j++
	}
	cmd := string(r[i:j])
	switch cmd {
	case `\frac`:
		num, j2 := latexGroup(r, j)
		den, j3 := latexGroup(r, j2)
		return num + "/" + den, j3
	case `\sqrt`:
		arg, j2 := latexGroup(r, j)
		return "√" + arg, j2
	}
	if latexPassthrough[cmd] {
		arg, j2 := latexGroup(r, j)
		return arg, j2
	}
	if u, ok := latexSym[cmd]; ok {
		return u, j
	}
	return cmd, j
}

func isLatexLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func mapScript(s string, table map[rune]rune, mark string) string {
	if s == "" {
		return ""
	}
	if strings.Contains(s, `\`) {
		return wrapScript(s, mark)
	}
	var b strings.Builder
	var gap strings.Builder
	flushGap := func() {
		if gap.Len() == 0 {
			return
		}
		b.WriteString(wrapScript(gap.String(), mark))
		gap.Reset()
	}
	for _, r := range s {
		if u, ok := table[r]; ok {
			flushGap()
			b.WriteRune(u)
			continue
		}
		gap.WriteRune(r)
	}
	flushGap()
	return b.String()
}

func wrapScript(s, mark string) string {
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) == 1 {
		return mark + s
	}
	return mark + "(" + s + ")"
}
