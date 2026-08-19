package tui

import (
	"fmt"
	"net/http"
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
	mdCaution
	mdBold
	mdItalic
	mdHR
	mdTable
	mdImage
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
		out = append(out, expandImages(expandEmphasis(parseInline(prose.String())))...)
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
		if cautionOpen(lines[i]) {
			flushProse()
			i++
			var body []string
			for i < len(lines) {
				text, ok := quoteLine(lines[i])
				if !ok {
					break
				}
				body = append(body, text)
				i++
			}
			out = append(out, mdSeg{kind: mdCaution, text: strings.Join(body, "\n")})
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

func cautionOpen(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, ">") {
		return false
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, ">"))
	return strings.EqualFold(s, "[!CAUTION]") || strings.EqualFold(s, "[!caution]")
}

func renderCaution(body string, inner int) string {
	barW := max(inner-2, 8)
	var lines []string
	lines = append(lines, stErr.Render("│ CAUTION"))
	for _, ln := range strings.Split(wrapVisual(body, barW), "\n") {
		lines = append(lines, stErr.Render("│ "+ln))
	}
	return strings.Join(lines, "\n")
}

func formatCautionMarkdown(err string) string {
	title := "Error"
	body := strings.TrimSpace(err)
	const prefix = "llm http "
	if strings.HasPrefix(err, prefix) {
		rest := err[len(prefix):]
		codeStr, msg, ok := strings.Cut(rest, ":")
		if ok {
			var code int
			if _, scanErr := fmt.Sscanf(strings.TrimSpace(codeStr), "%d", &code); scanErr == nil && code > 0 {
				title = fmt.Sprintf("HTTP %d", code)
				if st := http.StatusText(code); st != "" {
					title += " " + st
				}
				body = strings.TrimSpace(msg)
			}
		}
	}
	var b strings.Builder
	b.WriteString("> [!CAUTION]\n")
	b.WriteString("> " + title + "\n")
	if body == "" {
		return b.String()
	}
	for _, ln := range strings.Split(body, "\n") {
		b.WriteString("> " + ln + "\n")
	}
	return b.String()
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

func expandImages(segs []mdSeg) []mdSeg {
	var out []mdSeg
	for _, s := range segs {
		if s.kind == mdInline {
			if img, ok := exactLocalImage(s.text); ok {
				out = append(out, img)
				continue
			}
			out = append(out, s)
			continue
		}
		if s.kind != mdProse {
			out = append(out, s)
			continue
		}
		out = append(out, parseImages(s.text)...)
	}
	return out
}

func exactLocalImage(s string) (mdSeg, bool) {
	s = strings.TrimSpace(s)
	segs := parseImages(s)
	if len(segs) != 1 || segs[0].kind != mdImage {
		return mdSeg{}, false
	}
	return segs[0], true
}

func parseImages(s string) []mdSeg {
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
	for i := 0; i < len(r); {
		if r[i] != '!' || i+1 >= len(r) || r[i+1] != '[' {
			cur.WriteRune(r[i])
			i++
			continue
		}
		j := i + 2
		for j < len(r) && r[j] != ']' && r[j] != '\n' {
			j++
		}
		if j >= len(r) || r[j] != ']' || j+1 >= len(r) || r[j+1] != '(' {
			cur.WriteRune(r[i])
			i++
			continue
		}
		k := j + 2
		for k < len(r) && r[k] != ')' && r[k] != '\n' {
			k++
		}
		if k >= len(r) || r[k] != ')' {
			cur.WriteRune(r[i])
			i++
			continue
		}
		alt := string(r[i+2 : j])
		path := strings.TrimSpace(string(r[j+2 : k]))
		if path == "" || remoteImagePath(path) {
			cur.WriteRune(r[i])
			i++
			continue
		}
		flush()
		out = append(out, mdSeg{kind: mdImage, text: alt, lang: path})
		i = k + 1
	}
	flush()
	if len(out) == 0 {
		return []mdSeg{{kind: mdProse, text: s}}
	}
	return out
}

func remoteImagePath(p string) bool {
	l := strings.ToLower(strings.TrimSpace(p))
	return strings.Contains(l, "://") || strings.HasPrefix(l, "data:")
}

func imageAltFallback(alt string) string {
	alt = strings.TrimSpace(alt)
	if alt == "" {
		return "[image]"
	}
	return alt
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
	s, _ := renderRichOpts(text, inner, asst, richOpts{})
	return s
}

type richOpts struct {
	graphics bool
	live     bool
	maxRows  int
	resolve  func(string) (string, bool)
}

func renderRichOpts(text string, inner int, asst bool, opts richOpts) (string, []imgSlot) {
	segs := expandLatex(parseMarkdown(text))
	if len(segs) == 0 {
		if asst {
			return stAsstBubble.Render(""), nil
		}
		return stUserBubble.Render(""), nil
	}
	var blocks []string
	var run []mdSeg
	var slots []imgSlot
	used := 0
	add := func(block string) {
		if block == "" {
			return
		}
		blocks = append(blocks, block)
		used += strings.Count(block, "\n") + 1
	}
	paint := func(body string) {
		if asst {
			add(stAsstBubble.Render(body))
		} else {
			add(body)
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
		return renderInlineWrap(expandLatex(expandImages(expandEmphasis(parseInline(text)))), inner)
	}
	for _, s := range segs {
		switch s.kind {
		case mdFence:
			flush()
			add(renderFence(s, inner))
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
		case mdCaution:
			flush()
			add(renderCaution(s.text, inner))
		case mdHR:
			flush()
			paint(stMuted.Render(strings.Repeat("─", max(inner, 1))))
		case mdTable:
			flush()
			body, tslots := renderTable(s, inner, opts)
			for i := range tslots {
				tslots[i].contentLine += used
			}
			paint(body)
			slots = append(slots, tslots...)
		case mdImage:
			flush()
			body, slot := formatImageBlock(s, inner, opts)
			if body == "" {
				break
			}
			if slot != nil || strings.Contains(body, "▀") {
				if slot != nil {
					slot.contentLine = used
				}
				add(body)
				if slot != nil {
					slots = append(slots, *slot)
				}
				break
			}
			paint(stMuted.Render(body))
		default:
			run = append(run, s)
		}
	}
	flush()
	joined := strings.Join(blocks, "\n")
	if asst {
		return joined, slots
	}
	return stUserBubble.Render(joined), slots
}

func formatImageBlock(s mdSeg, inner int, opts richOpts) (string, *imgSlot) {
	fallback := imageAltFallback(s.text)
	abs, ok := "", false
	if opts.resolve != nil {
		abs, ok = opts.resolve(s.lang)
	}
	if !ok {
		return fallback, nil
	}
	if opts.live {
		if !opts.graphics {
			return "", nil
		}
		rows := opts.maxRows
		if rows < 1 {
			rows = 40
		}
		var b strings.Builder
		for i := 0; i < rows; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteByte(' ')
		}
		return b.String(), &imgSlot{
			path: s.lang,
			abs:  abs,
			cols: max(inner, 8),
			rows: rows,
			live: true,
		}
	}
	cols := inner
	if cols < 1 {
		cols = 1
	}
	maxRows := opts.maxRows
	if maxRows < 1 {
		maxRows = 40
	}
	preview, outCols, outRows := imagePreview(abs, cols, maxRows)
	if preview == "" {
		return fallback, nil
	}
	if !opts.graphics {
		return preview, nil
	}
	return preview, &imgSlot{
		path: s.lang,
		abs:  abs,
		cols: max(outCols, 1),
		rows: max(outRows, 1),
		live: false,
	}
}

func cellImageOnly(cell string) (mdSeg, bool) {
	segs := expandImages(expandEmphasis(parseInline(strings.TrimSpace(cell))))
	if len(segs) != 1 || segs[0].kind != mdImage {
		return mdSeg{}, false
	}
	return segs[0], true
}

func tableColStarts(widths []int) []int {
	starts := make([]int, len(widths))
	x := 0
	for i, w := range widths {
		starts[i] = x
		x += w
		if i+1 < len(widths) {
			x += 3
		}
	}
	return starts
}

func fitTableWidths(widths []int, imageCol []bool, inner int) {
	cols := len(widths)
	seps := 0
	if cols > 1 {
		seps = 3 * (cols - 1)
	}
	nImg := 0
	textSum := 0
	for i, w := range widths {
		if imageCol[i] {
			nImg++
		} else {
			textSum += w
		}
	}
	if nImg == 0 {
		total := textSum + seps
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
		return
	}
	remain := inner - seps - textSum
	need := nImg * 8
	for remain < need {
		k := -1
		for i := 0; i < cols; i++ {
			if imageCol[i] || widths[i] <= 1 {
				continue
			}
			if k < 0 || widths[i] > widths[k] {
				k = i
			}
		}
		if k < 0 {
			break
		}
		widths[k]--
		remain++
	}
	if remain < nImg {
		remain = nImg
	}
	base := remain / nImg
	extra := remain % nImg
	for i := range widths {
		if !imageCol[i] {
			continue
		}
		w := base
		if extra > 0 {
			w++
			extra--
		}
		if w < 1 {
			w = 1
		}
		widths[i] = w
	}
}

func renderTable(s mdSeg, inner int, opts richOpts) (string, []imgSlot) {
	rows := decodeTable(s.text)
	if len(rows) == 0 {
		return "", nil
	}
	cols := len(rows[0])
	imageCol := make([]bool, cols)
	for _, row := range rows[1:] {
		for i := 0; i < cols && i < len(row); i++ {
			if _, ok := cellImageOnly(row[i]); ok {
				imageCol[i] = true
			}
		}
	}
	inlineCell := func(text string) string {
		return renderInlineWrap(expandLatex(expandImages(expandEmphasis(parseInline(text)))), inner)
	}
	widths := make([]int, cols)
	for ri, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if ri > 0 {
				if _, ok := cellImageOnly(row[i]); ok {
					continue
				}
			}
			w := lipgloss.Width(stripANSI(firstLine(inlineCell(row[i]))))
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
	fitTableWidths(widths, imageCol, inner)
	starts := tableColStarts(widths)
	sep := stMuted.Render(" │ ")
	var slots []imgSlot
	var b strings.Builder
	writeCells := func(cells [][]string) {
		h := 1
		for _, c := range cells {
			if len(c) > h {
				h = len(c)
			}
		}
		for y := 0; y < h; y++ {
			if y > 0 {
				b.WriteByte('\n')
			}
			for i := 0; i < cols; i++ {
				if i > 0 {
					b.WriteString(sep)
				}
				line := ""
				if y < len(cells[i]) {
					line = cells[i][y]
				}
				b.WriteString(padVisual(line, widths[i]))
			}
		}
	}
	head := make([][]string, cols)
	for i := 0; i < cols; i++ {
		cell := ""
		if i < len(rows[0]) {
			cell = rows[0][i]
		}
		body := stHeading.Render(stripANSI(firstLine(inlineCell(cell))))
		head[i] = []string{body}
	}
	writeCells(head)
	b.WriteByte('\n')
	parts := make([]string, cols)
	for i, w := range widths {
		parts[i] = strings.Repeat("─", w)
	}
	b.WriteString(stMuted.Render(strings.Join(parts, "─┼─")))
	for _, row := range rows[1:] {
		b.WriteByte('\n')
		rowStart := strings.Count(b.String(), "\n")
		cells := make([][]string, cols)
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if img, ok := cellImageOnly(cell); ok {
				body, slot := formatImageBlock(img, widths[i], opts)
				if slot != nil {
					slot.col = starts[i]
					slot.contentLine = rowStart
					slots = append(slots, *slot)
				}
				if body == "" {
					cells[i] = []string{""}
					continue
				}
				if slot == nil && !strings.Contains(body, "▀") {
					cells[i] = []string{stMuted.Render(firstLine(body))}
					continue
				}
				cells[i] = strings.Split(body, "\n")
				continue
			}
			cells[i] = []string{firstLine(inlineCell(cell))}
		}
		writeCells(cells)
	}
	return b.String(), slots
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
		if s.kind == mdImage {
			writeAtomic(imageAltFallback(s.text), &stMuted)
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
