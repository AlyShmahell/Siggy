package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParseInlineCode(t *testing.T) {
	segs := parseMarkdown("a `code` b")
	if len(segs) != 3 {
		t.Fatalf("segs = %#v", segs)
	}
	if segs[0] != (mdSeg{kind: mdProse, text: "a "}) {
		t.Fatalf("prose = %#v", segs[0])
	}
	if segs[1] != (mdSeg{kind: mdInline, text: "code"}) {
		t.Fatalf("inline = %#v", segs[1])
	}
	if segs[2] != (mdSeg{kind: mdProse, text: " b"}) {
		t.Fatalf("trail = %#v", segs[2])
	}
}

func TestParseUnmatchedTickStays(t *testing.T) {
	segs := parseMarkdown("a ` b")
	if len(segs) != 1 || segs[0].kind != mdProse || segs[0].text != "a ` b" {
		t.Fatalf("segs = %#v", segs)
	}
}

func TestParseFencedGo(t *testing.T) {
	src := "intro\n```go\nfmt.Println(1)\n```\noutro"
	segs := parseMarkdown(src)
	if len(segs) != 3 {
		t.Fatalf("segs = %#v", segs)
	}
	if segs[0].kind != mdProse || segs[0].text != "intro" {
		t.Fatalf("intro = %#v", segs[0])
	}
	if segs[1].kind != mdFence || segs[1].lang != "go" || segs[1].text != "fmt.Println(1)" {
		t.Fatalf("fence = %#v", segs[1])
	}
	if segs[2].kind != mdProse || segs[2].text != "outro" {
		t.Fatalf("outro = %#v", segs[2])
	}
}

func TestParseUnclosedFence(t *testing.T) {
	segs := parseMarkdown("```python\nprint(1)")
	if len(segs) != 1 || segs[0].kind != mdFence || segs[0].lang != "python" || segs[0].text != "print(1)" {
		t.Fatalf("segs = %#v", segs)
	}
}

func TestRenderInlineHidesBackticks(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("a `code` b", 40, true))
	if strings.Contains(got, "`") {
		t.Fatalf("backticks leaked: %q", got)
	}
	if !strings.Contains(got, "code") {
		t.Fatalf("missing code: %q", got)
	}
}

func TestRenderFenceHidesMarkers(t *testing.T) {
	forceColor(t)
	src := "```go\nfmt.Println(1)\n```"
	got := stripANSI(renderRich(src, 40, true))
	if strings.Contains(got, "```") {
		t.Fatalf("fence leaked: %q", got)
	}
	if !strings.Contains(got, "go") {
		t.Fatalf("missing lang: %q", got)
	}
	if !strings.Contains(got, "fmt.Println(1)") {
		t.Fatalf("missing body: %q", got)
	}
}

func TestRenderUnclosedFence(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("```\nstill here", 40, true))
	if strings.Contains(got, "```") {
		t.Fatalf("fence leaked: %q", got)
	}
	if !strings.Contains(got, "still here") {
		t.Fatalf("missing body: %q", got)
	}
}

func TestRenderUnmatchedTickVisible(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("see ` foo", 40, true))
	if !strings.Contains(got, "`") {
		t.Fatalf("lost unmatched tick: %q", got)
	}
}

func TestRenderUnfencedMarkdown(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("# Title\n\n**bold** and `x`\n- item", 40, true))
	if !strings.Contains(got, "Title") {
		t.Fatalf("missing title: %q", got)
	}
	if strings.Contains(got, "# ") {
		t.Fatalf("heading marker leaked: %q", got)
	}
	if !strings.Contains(got, "bold") || strings.Contains(got, "**") {
		t.Fatalf("bold: %q", got)
	}
	if !strings.Contains(got, "item") {
		t.Fatalf("missing item: %q", got)
	}
	if strings.Contains(got, "`") {
		t.Fatalf("backticks leaked: %q", got)
	}
}

func TestRenderLineUserAndAsstMarkdown(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	user := m.renderLine(line{kind: "user", text: "run `ls`"}, 40)
	if strings.Contains(stripANSI(user), "`") {
		t.Fatalf("user backticks: %q", stripANSI(user))
	}
	asst := m.renderLine(line{kind: "asst-live", text: "```go\nx = 1\n```"}, 40)
	plain := stripANSI(asst)
	if strings.Contains(plain, "```") {
		t.Fatalf("asst fence: %q", plain)
	}
	if !strings.Contains(plain, "x = 1") {
		t.Fatalf("asst body: %q", plain)
	}
}

func TestRenderFenceGoChroma(t *testing.T) {
	forceColor(t)
	hi := renderFence(mdSeg{kind: mdFence, lang: "go", text: "fmt.Println(1)"}, 40)
	mono := renderFence(mdSeg{kind: mdFence, text: "fmt.Println(1)"}, 40)
	if !strings.Contains(stripANSI(hi), "fmt.Println") {
		t.Fatalf("missing text: %q", stripANSI(hi))
	}
	if strings.Count(hi, "\x1b[") <= strings.Count(mono, "\x1b[") {
		t.Fatalf("expected extra SGR from chroma\nhi=%q\nmono=%q", hi, mono)
	}
}

func TestRenderLatexAlpha(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich(`$\alpha$`, 40, true))
	if !strings.Contains(got, "α") {
		t.Fatalf("missing alpha: %q", got)
	}
	if strings.Contains(got, `\alpha`) {
		t.Fatalf("command leaked: %q", got)
	}
}

func TestRenderLatexFracGe(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich(`$m^* \ge T/h$ and $\frac{3}{2}$ and $t_{h+1}$`, 80, true))
	if strings.Contains(got, `\ge`) || strings.Contains(got, `\frac`) {
		t.Fatalf("command leaked: %q", got)
	}
	if !strings.Contains(got, "≥") {
		t.Fatalf("missing ge: %q", got)
	}
	if !strings.Contains(got, "3/2") {
		t.Fatalf("missing frac: %q", got)
	}
	if strings.Contains(got, "{3}") || strings.Contains(got, "{2}") {
		t.Fatalf("braces leaked: %q", got)
	}
}

func TestRenderLatexDoesNotSplitFrac(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich(`aaaaaa $\frac{3}{2}$`, 8, true))
	if strings.Contains(got, `\frac`) || strings.Contains(got, "{3}") {
		t.Fatalf("split source latex: %q", got)
	}
	if !strings.Contains(got, "3/2") {
		t.Fatalf("missing frac: %q", got)
	}
}

func TestRenderLatexDoesNotSpanNewline(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("$x\n\n# Title\n\n$y$", 80, true))
	if !strings.Contains(got, "Title") {
		t.Fatalf("missing title: %q", got)
	}
	plain := stripANSI(renderRich("# H\n\n- item\n\n**b**", 80, true))
	if !strings.Contains(plain, "H") || !strings.Contains(plain, "item") || !strings.Contains(plain, "b") {
		t.Fatalf("stream fragment: %q", plain)
	}
	if strings.Contains(plain, "**") || strings.Contains(plain, "# ") {
		t.Fatalf("markers leaked: %q", plain)
	}
}

func TestRenderLatexParenAndBare(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich(`\ge and \(\frac{3}{2}\)`, 80, true))
	if strings.Contains(got, `\ge`) || strings.Contains(got, `\frac`) {
		t.Fatalf("command leaked: %q", got)
	}
	if !strings.Contains(got, "≥") {
		t.Fatalf("missing ge: %q", got)
	}
	if !strings.Contains(got, "3/2") {
		t.Fatalf("missing frac: %q", got)
	}
}

func TestRenderLatexUnknownKept(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich(`$\foo$`, 40, true))
	if !strings.Contains(got, `\foo`) {
		t.Fatalf("unknown dropped: %q", got)
	}
}

func TestRenderLatexScripts(t *testing.T) {
	if got := renderLatex("t_{h+1}"); !strings.Contains(got, "ₕ₊₁") {
		t.Fatalf("sub group: %q", got)
	}
	if got := renderLatex("x^{n+1}"); !strings.Contains(got, "ⁿ⁺¹") {
		t.Fatalf("super group: %q", got)
	}
	got := renderLatex("x_i^2")
	if !strings.Contains(got, "ᵢ") || !strings.Contains(got, "²") {
		t.Fatalf("adjacent scripts: %q", got)
	}
	if strings.Contains(renderLatex("m^*"), "∗") {
		t.Fatalf("star became ast: %q", renderLatex("m^*"))
	}
	got = renderLatex("a^{bc}")
	if got == "abc" {
		t.Fatalf("super dropped: %q", got)
	}
	if !strings.Contains(got, "ᵇ") || !strings.Contains(got, "ᶜ") {
		t.Fatalf("super not mapped: %q", got)
	}
}

func TestRenderLineDiffColors(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	src := "edited a.go\n-    1 | old\n+    1 | new"
	got := m.renderLine(line{kind: "diff", text: src}, 40)
	if !strings.Contains(got, stErr.Render("-    1 | old")) {
		t.Fatalf("missing minus color: %q", got)
	}
	if !strings.Contains(got, stAdd.Render("+    1 | new")) {
		t.Fatalf("missing plus color: %q", got)
	}
}

func TestRenderHorizontalRule(t *testing.T) {
	forceColor(t)
	got := stripANSI(renderRich("a\n\n---\n\nb", 40, true))
	if strings.Contains(got, "---") {
		t.Fatalf("hyphens leaked: %q", got)
	}
	if !strings.Contains(got, "─") {
		t.Fatalf("missing rule: %q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("lost prose: %q", got)
	}
}

func TestRenderPipeTable(t *testing.T) {
	forceColor(t)
	src := "| Job | Time |\n|-----|------|\n| 1 | 3 |\n"
	got := stripANSI(renderRich(src, 80, true))
	if strings.Contains(got, "|-----") || strings.Contains(got, "|-----|") {
		t.Fatalf("sep leaked: %q", got)
	}
	if !strings.Contains(got, "Job") || !strings.Contains(got, "3") {
		t.Fatalf("missing cells: %q", got)
	}
	i := strings.Index(got, "Job")
	j := strings.Index(got, "Time")
	if i < 0 || j <= i {
		t.Fatalf("header order: %q", got)
	}
	between := got[i+3 : j]
	if !strings.Contains(between, "│") && !strings.Contains(between, "  ") {
		t.Fatalf("columns not aligned: %q", got)
	}
}

func TestTableSepIsNotHR(t *testing.T) {
	forceColor(t)
	src := "| Job | Time |\n|-----|------|\n| 1 | 3 |\n\n* item\n"
	got := stripANSI(renderRich(src, 80, true))
	if strings.Contains(got, "|-----") {
		t.Fatalf("table became source: %q", got)
	}
	if !strings.Contains(got, "Job") || !strings.Contains(got, "3") {
		t.Fatalf("missing table: %q", got)
	}
	if !strings.Contains(got, "item") {
		t.Fatalf("missing list: %q", got)
	}
	if strings.Contains(got, "* item") {
		t.Fatalf("list marker leaked: %q", got)
	}
}

func TestRenderTableImageCellMissingFile(t *testing.T) {
	forceColor(t)
	src := "| Name | Photo |\n|------|-------|\n| Duck | ![diagram](foo.png) |\n"
	got := stripANSI(renderRich(src, 80, true))
	if !strings.Contains(got, "Duck") || !strings.Contains(got, "diagram") {
		t.Fatalf("missing cells: %q", got)
	}
	if !strings.Contains(got, "│") {
		t.Fatalf("missing columns: %q", got)
	}
	if strings.Contains(got, "![") {
		t.Fatalf("markdown leaked: %q", got)
	}
	if strings.Contains(got, "▀") {
		t.Fatalf("preview without file: %q", got)
	}
}

func TestParseMarkdownImage(t *testing.T) {
	segs := parseMarkdown("see ![diagram](foo.png) here")
	var img mdSeg
	found := false
	for _, s := range segs {
		if s.kind == mdImage {
			img = s
			found = true
		}
	}
	if !found || img.text != "diagram" || img.lang != "foo.png" {
		t.Fatalf("segs = %#v", segs)
	}
	link := parseMarkdown("see [](foo.png)")
	for _, s := range link {
		if s.kind == mdImage {
			t.Fatalf("bare link became image: %#v", link)
		}
	}
	remote := parseMarkdown("![x](https://ex.com/a.png)")
	for _, s := range remote {
		if s.kind == mdImage {
			t.Fatalf("remote became image: %#v", remote)
		}
	}
	src := "```\n![x](foo.png)\n```"
	fences := parseMarkdown(src)
	if len(fences) != 1 || fences[0].kind != mdFence {
		t.Fatalf("fence = %#v", fences)
	}
	if !strings.Contains(fences[0].text, "![x](foo.png)") {
		t.Fatalf("fence body = %#v", fences[0])
	}
	tick := parseMarkdown("see `![diagram](foo.png)` here")
	found = false
	for _, s := range tick {
		if s.kind == mdImage {
			img = s
			found = true
		}
	}
	if !found || img.text != "diagram" || img.lang != "foo.png" {
		t.Fatalf("tick image = %#v", tick)
	}
	remoteTick := parseMarkdown("`![x](https://ex.com/a.png)`")
	for _, s := range remoteTick {
		if s.kind == mdImage {
			t.Fatalf("remote tick became image: %#v", remoteTick)
		}
	}
}

func TestRenderImageCaptionNoAPC(t *testing.T) {
	forceColor(t)
	got := renderRich("![diagram](foo.png)", 40, true)
	plain := stripANSI(got)
	if !strings.Contains(plain, "diagram") {
		t.Fatalf("missing alt: %q", plain)
	}
	if strings.Contains(plain, "![") {
		t.Fatalf("markdown leaked: %q", plain)
	}
	if strings.Contains(got, "\x1b_G") {
		t.Fatalf("APC leaked: %q", got)
	}
	empty := stripANSI(renderRich("![](foo.png)", 40, true))
	if !strings.Contains(empty, "[image]") {
		t.Fatalf("empty alt: %q", empty)
	}
}

func TestParseCautionBeforeQuote(t *testing.T) {
	src := "> [!CAUTION]\n> HTTP 401 Unauthorized\n> body text\n\n> a quote"
	segs := parseMarkdown(src)
	var caution, quote mdSeg
	for _, s := range segs {
		switch s.kind {
		case mdCaution:
			caution = s
		case mdQuote:
			quote = s
		}
	}
	if caution.kind != mdCaution || !strings.Contains(caution.text, "HTTP 401") || !strings.Contains(caution.text, "body text") {
		t.Fatalf("caution = %#v segs=%#v", caution, segs)
	}
	if quote.kind != mdQuote || quote.text != "a quote" {
		t.Fatalf("quote = %#v segs=%#v", quote, segs)
	}
	tight := parseMarkdown(">[!caution]\n> nope")
	if len(tight) == 0 || tight[0].kind != mdCaution {
		t.Fatalf("tight = %#v", tight)
	}
}

func TestRenderCautionHTTP401Wraps(t *testing.T) {
	forceColor(t)
	body := strings.Repeat("unauthorized-detail ", 30)
	src := formatCautionMarkdown("llm http 401: " + body)
	got := renderRich(src, 40, true)
	plain := stripANSI(got)
	if !strings.Contains(plain, "CAUTION") {
		t.Fatalf("missing caution: %q", plain)
	}
	if !strings.Contains(plain, "HTTP 401 Unauthorized") {
		t.Fatalf("missing title: %q", plain)
	}
	if strings.Contains(plain, "[!CAUTION]") {
		t.Fatalf("marker leaked: %q", plain)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected wrap: %q", plain)
	}
	for i, ln := range strings.Split(got, "\n") {
		if w := lipgloss.Width(ln); w > 40 {
			t.Fatalf("line %d width %d > 40: %q", i, w, ln)
		}
	}
}
