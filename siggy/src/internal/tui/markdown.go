package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type mdKind int

const (
	mdProse mdKind = iota
	mdInline
	mdFence
	mdLatex
	mdHeading
	mdList
	mdQuote
	mdBold
	mdItalic
	mdHR
	mdTable
)

type mdSeg struct {
	kind mdKind
	lang string
	text string
}

func parseMarkdown(s string) []mdSeg {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	var out []mdSeg
	var prose strings.Builder
	flushProse := func() {
		if prose.Len() == 0 {
			return
		}
		out = append(out, expandEmphasis(parseInline(prose.String()))...)
		prose.Reset()
	}
	for i := 0; i < len(lines); {
		if n, lang, ok := fenceOpen(lines[i]); ok {
			flushProse()
			i++
			var body []string
			for i < len(lines) {
				if fenceClose(lines[i], n) {
					i++
					break
				}
				body = append(body, lines[i])
				i++
			}
			out = append(out, mdSeg{kind: mdFence, lang: lang, text: strings.Join(body, "\n")})
			continue
		}
		if level, text, ok := headingLine(lines[i]); ok {
			flushProse()
			out = append(out, mdSeg{kind: mdHeading, lang: strings.Repeat("#", level), text: text})
			i++
			continue
		}
		if seg, next, ok := parseTable(lines, i); ok {
			flushProse()
			out = append(out, seg)
			i = next
			continue
		}
		if hrLine(lines[i]) {
			flushProse()
			out = append(out, mdSeg{kind: mdHR})
			i++
			continue
		}
		if marker, text, ok := listLine(lines[i]); ok {
			flushProse()
			out = append(out, mdSeg{kind: mdList, lang: marker, text: text})
			i++
			continue
		}
		if text, ok := quoteLine(lines[i]); ok {
			flushProse()
			out = append(out, mdSeg{kind: mdQuote, text: text})
			i++
			continue
		}
		if prose.Len() > 0 {
			prose.WriteByte('\n')
		}
		prose.WriteString(lines[i])
		i++
	}
	flushProse()
	return out
}

func hrLine(line string) bool {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return false
	}
	rest := strings.TrimSpace(line[indent:])
	if rest == "" || strings.ContainsAny(rest, "|") {
		return false
	}
	var b strings.Builder
	for _, r := range rest {
		if r == ' ' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	if len(s) < 3 {
		return false
	}
	mark := s[0]
	if mark != '-' && mark != '*' && mark != '_' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != mark {
			return false
		}
	}
	return true
}

func parseTable(lines []string, i int) (mdSeg, int, bool) {
	if i+1 >= len(lines) || !strings.Contains(lines[i], "|") || !isTableSep(lines[i+1]) {
		return mdSeg{}, i, false
	}
	sep := splitTableRow(lines[i+1])
	cols := len(sep)
	if cols == 0 {
		return mdSeg{}, i, false
	}
	rows := [][]string{fitTableCols(splitTableRow(lines[i]), cols)}
	j := i + 2
	for j < len(lines) {
		if strings.TrimSpace(lines[j]) == "" || !strings.Contains(lines[j], "|") || isTableSep(lines[j]) {
			break
		}
		if _, _, ok := headingLine(lines[j]); ok {
			break
		}
		if _, _, ok := fenceOpen(lines[j]); ok {
			break
		}
		rows = append(rows, fitTableCols(splitTableRow(lines[j]), cols))
		j++
	}
	return mdSeg{kind: mdTable, text: encodeTable(rows)}, j, true
}

func isTableSep(line string) bool {
	cells := splitTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !isTableSepCell(c) {
			return false
		}
	}
	return true
}

func isTableSepCell(c string) bool {
	c = strings.TrimSpace(c)
	if strings.HasPrefix(c, ":") {
		c = c[1:]
	}
	if strings.HasSuffix(c, ":") {
		c = c[:len(c)-1]
	}
	if len(c) < 3 {
		return false
	}
	for _, r := range c {
		if r != '-' {
			return false
		}
	}
	return true
}

func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	if s == "" {
		return nil
	}
	raw := strings.Split(s, "|")
	if strings.HasPrefix(s, "|") {
		raw = raw[1:]
	}
	if strings.HasSuffix(s, "|") && len(raw) > 0 {
		raw = raw[:len(raw)-1]
	}
	out := make([]string, len(raw))
	for i, c := range raw {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

func fitTableCols(cells []string, n int) []string {
	out := make([]string, n)
	for i := 0; i < n && i < len(cells); i++ {
		out[i] = cells[i]
	}
	return out
}

func encodeTable(rows [][]string) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, c := range row {
			if j > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(strings.ReplaceAll(c, "\t", " "))
		}
	}
	return b.String()
}

func decodeTable(s string) [][]string {
	lines := strings.Split(s, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

func headingLine(line string) (level int, text string, ok bool) {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return 0, "", false
	}
	rest := line[indent:]
	n := 0
	for n < len(rest) && rest[n] == '#' {
		n++
		if n > 6 {
			return 0, "", false
		}
	}
	if n == 0 {
		return 0, "", false
	}
	if n == len(rest) {
		return n, "", true
	}
	if rest[n] != ' ' && rest[n] != '\t' {
		return 0, "", false
	}
	text = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest[n:]), "#"))
	return n, text, true
}

func listLine(line string) (marker, text string, ok bool) {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return "", "", false
	}
	rest := line[indent:]
	switch {
	case strings.HasPrefix(rest, "- "), strings.HasPrefix(rest, "* "), strings.HasPrefix(rest, "+ "), strings.HasPrefix(rest, "• "):
		return "•", strings.TrimSpace(rest[2:]), true
	}
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(rest) && rest[i] == '.' && rest[i+1] == ' ' {
		return rest[:i] + ".", strings.TrimSpace(rest[i+2:]), true
	}
	return "", "", false
}

func quoteLine(line string) (text string, ok bool) {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return "", false
	}
	rest := line[indent:]
	if rest == ">" {
		return "", true
	}
	if strings.HasPrefix(rest, "> ") {
		return rest[2:], true
	}
	return "", false
}

func fenceOpen(line string) (n int, lang string, ok bool) {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return 0, "", false
	}
	rest := line[indent:]
	n = countTicks(rest)
	if n < 3 {
		return 0, "", false
	}
	info := strings.TrimSpace(rest[n:])
	if strings.Contains(info, "`") {
		return 0, "", false
	}
	if f := strings.Fields(info); len(f) > 0 {
		lang = f[0]
	}
	return n, lang, true
}

func fenceClose(line string, n int) bool {
	indent := leadingSpaces(line, 3)
	if indent < 0 {
		return false
	}
	rest := line[indent:]
	m := countTicks(rest)
	if m < n {
		return false
	}
	return strings.TrimSpace(rest[m:]) == ""
}

func leadingSpaces(line string, maxN int) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
		if n > maxN {
			return -1
		}
	}
	return n
}

func countTicks(s string) int {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	return n
}

func parseInline(s string) []mdSeg {
	if s == "" {
		return nil
	}
	var out []mdSeg
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, mdSeg{kind: mdProse, text: cur.String()})
		cur.Reset()
	}
	r := []rune(s)
	for i := 0; i < len(r); {
		if r[i] != '`' {
			cur.WriteRune(r[i])
			i++
			continue
		}
		n := 1
		for i+n < len(r) && r[i+n] == '`' {
			n++
		}
		if n >= 3 {
			cur.WriteString(string(r[i : i+n]))
			i += n
			continue
		}
		j := i + n
		for j < len(r) && r[j] != '`' {
			j++
		}
		if j >= len(r) {
			cur.WriteString(string(r[i : i+n]))
			i += n
			continue
		}
		flush()
		out = append(out, mdSeg{kind: mdInline, text: string(r[i+n : j])})
		i = j + 1
	}
	flush()
	return out
}

func expandEmphasis(segs []mdSeg) []mdSeg {
	var out []mdSeg
	for _, s := range segs {
		if s.kind != mdProse {
			out = append(out, s)
			continue
		}
		out = append(out, parseEmphasis(s.text)...)
	}
	return out
}

func parseEmphasis(s string) []mdSeg {
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
	isWord := func(ch rune) bool {
		return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
	}
	for i := 0; i < len(r); {
		if r[i] != '*' && r[i] != '_' {
			cur.WriteRune(r[i])
			i++
			continue
		}
		delim := r[i]
		n := 1
		if i+1 < len(r) && r[i+1] == delim {
			n = 2
		}
		j := i + n
		for j < len(r) {
			if n == 2 && r[j] == delim && j+1 < len(r) && r[j+1] == delim {
				break
			}
			if n == 1 && r[j] == delim && (j+1 >= len(r) || r[j+1] != delim) {
				break
			}
			j++
		}
		if j >= len(r) {
			cur.WriteString(string(r[i : i+n]))
			i += n
			continue
		}
		inner := string(r[i+n : j])
		if strings.TrimSpace(inner) == "" {
			cur.WriteString(string(r[i : j+n]))
			i = j + n
			continue
		}
		if delim == '_' && n == 1 {
			if i > 0 && isWord(r[i-1]) && j+n < len(r) && isWord(r[j+n]) {
				cur.WriteRune(r[i])
				i++
				continue
			}
		}
		flush()
		kind := mdItalic
		if n == 2 {
			kind = mdBold
		}
		out = append(out, mdSeg{kind: kind, text: inner})
		i = j + n
	}
	flush()
	if len(out) == 0 {
		return []mdSeg{{kind: mdProse, text: s}}
	}
	return out
}

func renderRich(text string, inner int, asst bool) string {
	segs := expandLatex(parseMarkdown(text))
	if len(segs) == 0 {
		if asst {
			return stAsstBubble.Render("")
		}
		return stUserBubble.Render("")
	}
	var blocks []string
	var run []mdSeg
	paint := func(body string) {
		if asst {
			blocks = append(blocks, stAsstBubble.Render(body))
		} else {
			blocks = append(blocks, body)
		}
	}
	flush := func() {
		if len(run) == 0 {
			return
		}
		paint(renderInlineWrap(run, inner))
		run = nil
	}
	inline := func(text string) string {
		return renderInlineWrap(expandLatex(expandEmphasis(parseInline(text))), inner)
	}
	for _, s := range segs {
		switch s.kind {
		case mdFence:
			flush()
			blocks = append(blocks, renderFence(s, inner))
		case mdHeading:
			flush()
			paint(stHeading.Render(inline(s.text)))
		case mdList:
			flush()
			marker := s.lang
			if marker == "" {
				marker = "•"
			}
			paint(stMuted.Render(marker+" ") + inline(s.text))
		case mdQuote:
			flush()
			paint(stQuote.Render("│ " + inline(s.text)))
		case mdHR:
			flush()
			paint(stMuted.Render(strings.Repeat("─", max(inner, 1))))
		case mdTable:
			flush()
			paint(renderTable(s, inner, inline))
		default:
			run = append(run, s)
		}
	}
	flush()
	joined := strings.Join(blocks, "\n")
	if asst {
		return joined
	}
	return stUserBubble.Render(joined)
}

func renderTable(s mdSeg, inner int, inline func(string) string) string {
	rows := decodeTable(s.text)
	if len(rows) == 0 {
		return ""
	}
	cols := len(rows[0])
	widths := make([]int, cols)
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			w := lipgloss.Width(stripANSI(inline(row[i])))
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	sepW := 3
	total := 0
	for _, w := range widths {
		total += w
	}
	if cols > 1 {
		total += sepW * (cols - 1)
	}
	for total > inner && inner > 0 {
		k := 0
		for i := 1; i < cols; i++ {
			if widths[i] > widths[k] {
				k = i
			}
		}
		if widths[k] <= 1 {
			break
		}
		widths[k]--
		total--
	}
	var b strings.Builder
	writeRow := func(row []string, head bool) {
		for i := 0; i < cols; i++ {
			if i > 0 {
				b.WriteString(stMuted.Render(" │ "))
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			body := firstLine(renderInlineWrap(expandLatex(expandEmphasis(parseInline(cell))), widths[i]))
			if head {
				body = stHeading.Render(stripANSI(body))
			}
			b.WriteString(padVisual(body, widths[i]))
		}
	}
	if len(rows) > 0 {
		writeRow(rows[0], true)
		b.WriteByte('\n')
		parts := make([]string, cols)
		for i, w := range widths {
			parts[i] = strings.Repeat("─", w)
		}
		b.WriteString(stMuted.Render(strings.Join(parts, "─┼─")))
		for _, row := range rows[1:] {
			b.WriteByte('\n')
			writeRow(row, false)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func padVisual(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func renderFence(s mdSeg, inner int) string {
	w := max(inner, 8)
	codeW := max(w-2, 4)
	raw := s.text
	if colored := highlightCode(s.lang, s.text); colored != "" {
		raw = colored
	}
	body := stCodeBlock.Render(padCodeLines(wrapVisual(raw, codeW), codeW))
	if s.lang == "" {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, stCodeLang.Render(s.lang), body)
}

func padCodeLines(s string, w int) string {
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		pad := max(w-lipgloss.Width(ln), 0)
		lines = append(lines, ln+strings.Repeat(" ", pad))
	}
	return strings.Join(lines, "\n")
}

func renderInlineWrap(segs []mdSeg, inner int) string {
	if inner < 1 {
		inner = 1
	}
	var lines []string
	var buf strings.Builder
	width := 0
	flush := func() {
		lines = append(lines, buf.String())
		buf.Reset()
		width = 0
	}
	write := func(text string, st *lipgloss.Style) {
		r := []rune(text)
		for len(r) > 0 {
			room := inner - width
			if room <= 0 {
				flush()
				room = inner
			}
			n := len(r)
			if n > room {
				n = room
			}
			chunk := string(r[:n])
			r = r[n:]
			if st != nil {
				buf.WriteString(st.Render(chunk))
			} else {
				buf.WriteString(chunk)
			}
			width += n
		}
	}
	writeAtomic := func(text string, st *lipgloss.Style) {
		w := lipgloss.Width(text)
		if w == 0 {
			return
		}
		if width > 0 && width+w > inner {
			flush()
		}
		if w <= inner {
			if st != nil {
				buf.WriteString(st.Render(text))
			} else {
				buf.WriteString(text)
			}
			width += w
			return
		}
		write(text, st)
	}
	for _, s := range segs {
		if s.kind == mdProse {
			parts := strings.Split(s.text, "\n")
			for i, p := range parts {
				if i > 0 {
					flush()
				}
				write(p, nil)
			}
			continue
		}
		if s.kind == mdLatex {
			if s.lang == "display" && width > 0 {
				flush()
			}
			writeAtomic(renderLatex(s.text), &stLatex)
			if s.lang == "display" {
				flush()
			}
			continue
		}
		switch s.kind {
		case mdBold:
			write(s.text, &stBold)
		case mdItalic:
			write(s.text, &stItalic)
		default:
			write(s.text, &stInlineCode)
		}
	}
	if buf.Len() > 0 || len(lines) == 0 {
		flush()
	}
	return strings.Join(lines, "\n")
}
