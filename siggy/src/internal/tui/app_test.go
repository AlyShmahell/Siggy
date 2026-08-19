package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"siggy/src/internal/config"
	"siggy/src/internal/graph"
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
	"siggy/src/internal/tools"
	"siggy/src/internal/version"
)

func testModel(t *testing.T) model {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Home:           home,
		Workspace:      root,
		ActiveProvider: "env",
		Model:          "gpt-4.1",
		ContextWindow:  128000,
		Providers: []config.Provider{{
			Name: "env", URL: "http://127.0.0.1:8080", Models: []string{"gpt-4.1"}, Protocols: []string{config.ProtocolOpenAI},
		}},
	}
	g := graph.New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	m := New(g, h, cfg)
	m.width, m.height = 80, 24
	m.layout()
	m.refresh()
	_ = m.paint()
	return m
}

func TestNewRenders(t *testing.T) {
	m := testModel(t)
	view := m.View()
	if !strings.Contains(view, "siggy") {
		t.Fatalf("view missing brand: %q", view)
	}
	nav := strings.Split(stripANSI(view), "\n")[0]
	if !strings.Contains(nav, "siggy") {
		t.Fatalf("navbar missing brand: %q", nav)
	}
	for _, ln := range m.lines {
		if strings.Contains(strings.ToLower(ln.text), "siggy") {
			t.Fatalf("transcript still has brand: %#v", ln)
		}
	}
}

func TestHitMapStopAndProviders(t *testing.T) {
	m := testModel(t)
	stop, ok := firstKind(&m, KindCancel)
	if !ok {
		t.Fatal("missing stop target")
	}
	got, ok := m.hits.At(stop.Rect.X, stop.Rect.Y)
	if !ok || got.Kind != KindCancel {
		t.Fatalf("stop hit = %#v ok=%v", got, ok)
	}
	next, _ := m.activate(stop)
	nm := next.(model)
	if nm.running {
		t.Fatal("idle stop should be a no-op")
	}

	gear, ok := firstKind(&m, KindNavGear)
	if !ok {
		t.Fatal("missing gear")
	}
	next, _ = m.activate(gear)
	nm = next.(model)
	if nm.page != pageSettings {
		t.Fatalf("page = %d", nm.page)
	}
}

func TestEnterSubmits(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("hello")
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	found := false
	for _, ln := range nm.lines {
		if ln.kind == "user" && ln.text == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("submit did not record user line: %#v", nm.lines)
	}
}

func TestShiftEnterNewline(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("ab")
	m.selCaret = 1
	m.selAnchor = 1
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	nm := next.(model)
	if !strings.Contains(nm.ta.Value(), "\n") {
		t.Fatalf("ctrl+j expected newline, got %q", nm.ta.Value())
	}
	if nm.running {
		t.Fatal("newline should not submit")
	}
	m = testModel(t)
	m.ta.SetValue("hi")
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	nm = next.(model)
	if !strings.Contains(nm.ta.Value(), "\n") {
		t.Fatalf("alt+enter expected newline, got %q", nm.ta.Value())
	}
}

func TestModeChipToggles(t *testing.T) {
	m := testModel(t)
	mode, ok := firstKind(&m, KindComposerMode)
	if !ok {
		t.Fatal("no mode chip")
	}
	next, _ := m.activate(mode)
	nm := next.(model)
	if nm.float != floatMode {
		t.Fatalf("float = %d", nm.float)
	}
	_ = nm.View()
	planIdx := -1
	for i, it := range modeItems {
		if it == string(harness.ModePlan) {
			planIdx = i
		}
	}
	if planIdx < 0 {
		t.Fatal("plan missing from mode menu")
	}
	next, _ = nm.activate(Target{Kind: KindModalItem, Index: planIdx})
	nm = next.(model)
	if nm.g.Engine.Harness.Mode != harness.ModePlan {
		t.Fatalf("mode = %s", nm.g.Engine.Harness.Mode)
	}
	if nm.page != pageSession {
		t.Fatalf("page = %d", nm.page)
	}
}

func TestApprovalRectsInsideModal(t *testing.T) {
	m := testModel(t)
	m.approval = &harness.ApprovalRequest{Tool: "write_file", Risk: "write", Summary: "x", Reply: make(chan harness.Decision, 1)}
	_ = m.View()
	if m.reg.modal.W == 0 {
		t.Fatal("modal rect empty")
	}
	items := m.hits.OfKind(KindApprove)
	if len(items) != 3 {
		t.Fatalf("approve targets = %d", len(items))
	}
	for _, it := range items {
		if !m.reg.modal.Contains(it.Rect.X, it.Rect.Y) {
			t.Fatalf("approve %#v outside modal %#v", it.Rect, m.reg.modal)
		}
	}
	outsideX := m.reg.transcript.X
	outsideY := m.reg.transcript.Y
	if m.reg.modal.Contains(outsideX, outsideY) {
		outsideY = m.reg.modal.Y - 1
		if outsideY < m.reg.transcript.Y {
			outsideX = m.reg.transcript.X
			outsideY = m.reg.transcript.Y
		}
	}
	got, ok := m.hits.At(outsideX, outsideY)
	if ok && got.Kind == KindApprove {
		t.Fatal("outside click hit approve")
	}
	if !ok || got.Kind != KindModalDismiss && got.Kind != KindTranscript && got.Kind != KindNone {
		// dismiss is layered under modal; a point in transcript but not modal should be dismiss
		if t2, ok2 := m.hits.At(m.reg.transcript.X, m.reg.transcript.Y); !ok2 || (t2.Kind != KindModalDismiss && t2.Kind != KindTranscript) {
			t.Fatalf("expected dismiss/transcript at corner, got %#v ok=%v (at=%#v ok=%v)", t2, ok2, got, ok)
		}
	}
}

func TestApprovalAllowOnceClosesAndLogs(t *testing.T) {
	m := testModel(t)
	reply := make(chan harness.Decision, 1)
	m.approval = &harness.ApprovalRequest{Tool: "write_file", Risk: "write", Summary: "fibonacci.py", Reply: reply}
	m.choice = 0
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if nm.approval != nil {
		t.Fatal("approval still set")
	}
	_ = nm.View()
	if items := nm.hits.OfKind(KindApprove); len(items) != 0 {
		t.Fatalf("approve hits = %d", len(items))
	}
	select {
	case d := <-reply:
		if d != harness.AllowOnce {
			t.Fatalf("decision = %v", d)
		}
	default:
		t.Fatal("no decision on Reply")
	}
	if !hasLine(nm.lines, "sys", "approve write_file (write): fibonacci.py") {
		t.Fatalf("missing Q: %#v", nm.lines)
	}
	if !hasLine(nm.lines, "sys", "→ allow once") {
		t.Fatalf("missing A: %#v", nm.lines)
	}
}

func TestApprovalClickClosesAndLogs(t *testing.T) {
	m := testModel(t)
	reply := make(chan harness.Decision, 1)
	m.approval = &harness.ApprovalRequest{Tool: "write_file", Risk: "write", Summary: "x", Reply: reply}
	next, _ := m.activate(Target{Kind: KindApprove, Index: 1})
	nm := next.(model)
	if nm.approval != nil {
		t.Fatal("approval still set")
	}
	select {
	case d := <-reply:
		if d != harness.AllowSession {
			t.Fatalf("decision = %v", d)
		}
	default:
		t.Fatal("no decision on Reply")
	}
	if !hasLine(nm.lines, "sys", "→ allow session") {
		t.Fatalf("missing A: %#v", nm.lines)
	}
}

func TestApprovalEscDeniesAndLogs(t *testing.T) {
	m := testModel(t)
	reply := make(chan harness.Decision, 1)
	m.approval = &harness.ApprovalRequest{Tool: "write_file", Risk: "write", Summary: "x", Reply: reply}
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)
	if nm.approval != nil {
		t.Fatal("approval still set")
	}
	_ = nm.View()
	if items := nm.hits.OfKind(KindApprove); len(items) != 0 {
		t.Fatalf("approve hits = %d", len(items))
	}
	select {
	case d := <-reply:
		if d != harness.Deny {
			t.Fatalf("decision = %v", d)
		}
	default:
		t.Fatal("no decision on Reply")
	}
	if !hasLine(nm.lines, "sys", "→ deny") {
		t.Fatalf("missing A: %#v", nm.lines)
	}
}

func hasLine(lines []line, kind, text string) bool {
	for _, ln := range lines {
		if ln.kind == kind && ln.text == text {
			return true
		}
	}
	return false
}

func TestHoverSelectsListIndex(t *testing.T) {
	m := testModel(t)
	m.openFloat(floatMode)
	_ = m.View()
	items := m.hits.OfKind(KindModalItem)
	if len(items) < 2 {
		t.Fatalf("palette items = %d", len(items))
	}
	m.hoverSelect(items[1])
	if m.palIdx != 1 {
		t.Fatalf("palIdx = %d", m.palIdx)
	}
}

func TestProviderFormSave(t *testing.T) {
	m := testModel(t)
	t.Setenv("SIGGY_HOME", m.cfg.Home)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("SIGGY_WORKSPACE", m.cfg.Workspace)
	m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: []string{""}})
	m.form.name = "work"
	m.form.url = "https://api.example/v1"
	m.form.apiKey = "sk-test-key"
	m.form.models = []string{"gpt-4.1"}
	m.form.protocols = []string{config.ProtocolOpenAI}
	if err := m.saveForm(); err != nil {
		t.Fatal(err)
	}
	p := m.cfg.Provider("work")
	if p == nil || p.URL != "https://api.example/v1" || p.APIKey != "sk-test-key" {
		t.Fatalf("saved %#v", p)
	}
	again, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := again.Provider("work")
	if got == nil || got.URL != p.URL || got.APIKey != p.APIKey || !containsStr(got.Protocols, config.ProtocolOpenAI) {
		t.Fatalf("load %#v", got)
	}
}

func TestProvidersHitMatchesGlyph(t *testing.T) {
	m := testModel(t)
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	view := m.View()
	y, row, ok := findPlainRow(view, "Providers")
	if !ok {
		t.Fatalf("Providers not visible in\n%s", stripANSI(view))
	}
	x := strings.Index(row, "Providers")
	got, ok := m.hits.At(x, y)
	if !ok || got.Kind != KindSidebarProviders {
		t.Fatalf("click on Providers row y=%d x=%d hit %#v ok=%v", y, x, got, ok)
	}
	if y >= 2 {
		miss, hit := m.hits.At(x, y-2)
		if hit && miss.Kind == KindSidebarProviders {
			t.Fatalf("click 2 rows above Providers still hit %#v", miss)
		}
	}
}

func TestVersionHitMatchesGlyph(t *testing.T) {
	m := testModel(t)
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	view := m.View()
	y, row, ok := findPlainRow(view, "Version")
	if !ok {
		t.Fatalf("Version not visible in\n%s", stripANSI(view))
	}
	x := strings.Index(row, "Version")
	got, ok := m.hits.At(x, y)
	if !ok || got.Kind != KindSidebarVersion {
		t.Fatalf("click on Version row y=%d x=%d hit %#v ok=%v", y, x, got, ok)
	}
}

func TestSettingsSidenavHeaderAndRules(t *testing.T) {
	m := testModel(t)
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "settings") || !strings.Contains(plain, glyphBack) {
		t.Fatalf("missing header: %q", plain)
	}
	y, _, ok := findPlainRow(view, "Providers")
	if !ok {
		t.Fatal("Providers missing")
	}
	vy, _, ok := findPlainRow(view, "Version")
	if !ok {
		t.Fatal("Version missing")
	}
	if vy <= y {
		t.Fatalf("Version should be below Providers: %d %d", y, vy)
	}
	sep := strings.Split(plain, "\n")
	foundRule := false
	for i := y + 1; i < vy && i < len(sep); i++ {
		if strings.Contains(sep[i], "─") {
			foundRule = true
			break
		}
	}
	if !foundRule {
		t.Fatalf("missing rule between Providers and Version:\n%s", plain)
	}
	back, ok := firstKind(&m, KindFormBack)
	if !ok {
		t.Fatal("missing back")
	}
	next, _ = m.activate(back)
	nm := next.(model)
	if nm.page != pageSession {
		t.Fatalf("page = %d", nm.page)
	}
}

func TestProvidersPlusAndRowActions(t *testing.T) {
	m := testModel(t)
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	m.cfg.Providers = []config.Provider{
		{Name: "env", URL: "http://127.0.0.1:8080", Models: []string{"gpt-4.1"}, Protocols: []string{config.ProtocolOpenAI}},
		{Name: "work", URL: "http://127.0.0.1:8081", Models: []string{"local"}, Protocols: []string{config.ProtocolOpenAI}},
	}
	_ = m.View()
	plain := stripANSI(m.View())
	for i, line := range strings.Split(plain, "\n") {
		if i == 0 {
			continue
		}
		if strings.Contains(line, " new ") || strings.Contains(line, " edit ") || strings.Contains(line, " back ") {
			t.Fatalf("bottom buttons still present:\n%s", plain)
		}
	}
	plus, ok := firstKind(&m, KindProviderNew)
	if !ok {
		t.Fatal("missing +")
	}
	row, ok := firstKind(&m, KindProviderRow)
	if !ok {
		t.Fatal("missing provider row")
	}
	if plus.Rect.Y >= row.Rect.Y {
		t.Fatalf("+ should be above providers: plus=%d row=%d", plus.Rect.Y, row.Rect.Y)
	}
	edit, ok := firstKind(&m, KindProviderEdit)
	if !ok || edit.Index != 0 {
		t.Fatalf("edit = %#v ok=%v", edit, ok)
	}
	del, ok := firstKind(&m, KindProviderDelete)
	if !ok || del.Index != 0 {
		t.Fatalf("delete = %#v ok=%v", del, ok)
	}
	if !(edit.Rect.X < del.Rect.X && del.Rect.X < row.Rect.X) {
		t.Fatalf("icons should be left of name: edit=%d del=%d row=%d", edit.Rect.X, del.Rect.X, row.Rect.X)
	}
	got, ok := m.hits.At(edit.Rect.X, edit.Rect.Y)
	if !ok || got.Kind != KindProviderEdit || got.Index != 0 {
		t.Fatalf("click ✎ hit %#v ok=%v", got, ok)
	}
	got, ok = m.hits.At(del.Rect.X, del.Rect.Y)
	if !ok || got.Kind != KindProviderDelete || got.Index != 0 {
		t.Fatalf("click ✕ hit %#v ok=%v", got, ok)
	}
	got, ok = m.hits.At(row.Rect.X, row.Rect.Y)
	if !ok || got.Kind != KindProviderRow || got.Index != 0 {
		t.Fatalf("click name hit %#v ok=%v", got, ok)
	}
	next, _ = m.activate(edit)
	nm := next.(model)
	if nm.page != pageProviderForm || nm.form.original != "env" {
		t.Fatalf("edit page=%d original=%q", nm.page, nm.form.original)
	}
	next, _ = nm.activate(Target{Kind: KindFormBack})
	nm = next.(model)
	next, _ = nm.activate(del)
	nm = next.(model)
	if len(nm.cfg.Providers) != 1 || nm.cfg.Providers[0].Name != "work" {
		t.Fatalf("providers after delete = %#v", nm.cfg.Providers)
	}
}

func TestFormSaveHitMatchesGlyph(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: []string{""}})
	view := m.View()
	y, row, ok := findPlainRow(view, "save")
	if !ok {
		t.Fatalf("save not visible in\n%s", stripANSI(view))
	}
	x := strings.Index(row, "save")
	got, ok := m.hits.At(x, y)
	if !ok || got.Kind != KindFormSave {
		t.Fatalf("click on save row y=%d x=%d hit %#v ok=%v", y, x, got, ok)
	}
	if y >= 3 {
		miss, hit := m.hits.At(x, y-3)
		if hit && miss.Kind == KindFormSave {
			t.Fatalf("click 3 rows above save still hit %#v", miss)
		}
	}
}

func TestComposerChipsAndModelMenu(t *testing.T) {
	m := testModel(t)
	m.cfg.Providers = []config.Provider{
		{Name: "env", URL: "http://127.0.0.1:8080", Models: []string{"gpt-4.1", "mini"}, Protocols: []string{config.ProtocolOpenAI}},
		{Name: "work", URL: "http://127.0.0.1:8081", Models: []string{"local"}, Protocols: []string{config.ProtocolOpenAI}},
	}
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "/ commands") {
		t.Fatal("commands palette chip still visible")
	}
	if strings.Contains(plain, "Think") {
		t.Fatalf("idle composer still shows Think:\n%s", plain)
	}
	for _, leak := range []string{"enter  esc", "arrows/enter/esc", "n new"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("keyboard hint %q still visible:\n%s", leak, plain)
		}
	}
	_, row, ok := findPlainRow(view, "act")
	if !ok {
		t.Fatalf("mode chip missing:\n%s", plain)
	}
	ri, ai := strings.Index(row, "ready"), strings.Index(row, "act")
	mi, hi := strings.Index(row, "env /"), strings.Index(row, "…")
	if ri < 0 || ai < 0 || ri > ai {
		t.Fatalf("phase should be left of mode on %q", row)
	}
	if mi < 0 || mi < ai {
		t.Fatalf("model chip should sit on the hint row after mode: %q", row)
	}
	if hi < 0 || hi < mi {
		t.Fatalf("health should sit next to the model chip: %q", row)
	}
	chip, ok := firstKind(&m, KindComposerModel)
	if !ok {
		t.Fatal("no model chip")
	}
	if chip.Rect.X >= m.width/2 {
		t.Fatalf("model chip should be on the left half: %#v", chip.Rect)
	}
	if _, ok := firstKind(&m, KindSend); ok {
		t.Fatal("send button should be gone")
	}
	plain = stripANSI(m.View())
	if !strings.Contains(plain, usageBadge(m.usageUsed(), m.contextWindow())) {
		t.Fatalf("usage badge missing:\n%s", plain)
	}
	next, _ := m.activate(chip)
	nm := next.(model)
	if nm.float != floatModel {
		t.Fatalf("float = %d", nm.float)
	}
	if nm.page != pageSession {
		t.Fatal("model menu left session page")
	}
	_ = nm.View()
	pairs := nm.modelPairs()
	idx := -1
	for i, p := range pairs {
		if p.provider == "work" && p.model == "local" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("pairs = %#v", pairs)
	}
	next, _ = nm.activate(Target{Kind: KindModalItem, Index: idx})
	nm = next.(model)
	if nm.cfg.ActiveProvider != "work" || nm.cfg.Model != "local" {
		t.Fatalf("active=%q model=%q", nm.cfg.ActiveProvider, nm.cfg.Model)
	}
	if nm.page != pageSession {
		t.Fatalf("page = %d", nm.page)
	}
	if nm.float != floatNone {
		t.Fatal("menu still open")
	}
}

func TestModelHealthFromLLM(t *testing.T) {
	m := testModel(t)
	_, row, ok := findPlainRow(m.View(), "act")
	if !ok || !strings.Contains(row, "…") {
		t.Fatalf("idle health should be unverified: %q", row)
	}
	next, _ := m.Update(healthMsg{status: "ok"})
	nm := next.(model)
	_, row, ok = findPlainRow(nm.View(), "act")
	if !ok || !strings.Contains(row, "ok") {
		t.Fatalf("probe ok should set health: %q", row)
	}
	next, _ = nm.onEvent(loop.Event{Kind: loop.KindError, Err: errors.New("llm http 401")})
	nm = next.(model)
	_, row, ok = findPlainRow(nm.View(), "act")
	if !ok || !strings.Contains(row, "err") {
		t.Fatalf("llm error should set health err: %q", row)
	}
	next, _ = nm.onEvent(loop.Event{Kind: loop.KindDone})
	nm = next.(model)
	_, row, ok = findPlainRow(nm.View(), "act")
	if !ok || !strings.Contains(row, "ok") {
		t.Fatalf("done should set health ok: %q", row)
	}
	next, _ = nm.onEvent(loop.Event{Kind: loop.KindError, Tool: "bash", Err: errors.New("denied")})
	nm = next.(model)
	if nm.modelHealth != "ok" {
		t.Fatalf("tool error changed health: %q", nm.modelHealth)
	}
	_, row, ok = findPlainRow(nm.View(), "act")
	if !ok || !strings.Contains(row, "ok") {
		t.Fatalf("tool error should keep chip health ok: %q", row)
	}
}

func TestFloatMenuScrolls(t *testing.T) {
	m := testModel(t)
	var models []string
	for i := 0; i < 30; i++ {
		models = append(models, fmt.Sprintf("zz-%02d", i))
	}
	m.cfg.Providers = []config.Provider{{
		Name: "env", URL: "http://127.0.0.1:8080", Models: models, Protocols: []string{config.ProtocolOpenAI},
	}}
	m.openFloat(floatModel)
	plain := stripANSI(m.View())
	if strings.Contains(plain, "zz-29") {
		t.Fatalf("last model should be clipped before scroll:\n%s", plain)
	}
	item, ok := firstKind(&m, KindModalItem)
	if !ok {
		t.Fatal("no modal items")
	}
	for i := 0; i < 25; i++ {
		next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: item.Rect.X, Y: item.Rect.Y})
		m = next.(model)
		_ = m.View()
		item, ok = firstKind(&m, KindModalItem)
		if !ok {
			t.Fatal("modal items disappeared")
		}
	}
	plain = stripANSI(m.View())
	if !strings.Contains(plain, "zz-29") {
		t.Fatalf("wheel should reveal later model:\n%s", plain)
	}
}

func TestTranscriptStickyBottom(t *testing.T) {
	m := testModel(t)
	for i := 0; i < 80; i++ {
		m.lines = append(m.lines, line{kind: "sys", text: fmt.Sprintf("row-%02d", i)})
	}
	m.followBottom = true
	m.refresh()
	if !m.vp.AtBottom() {
		t.Fatal("expected to follow bottom")
	}
	m.vp.GotoTop()
	if m.vp.AtBottom() {
		t.Fatal("GotoTop left viewport at bottom")
	}
	m.lines = append(m.lines, line{kind: "sys", text: "new-tail"})
	m.refresh()
	if m.vp.AtBottom() {
		t.Fatal("refresh while scrolled up jumped to bottom")
	}
	body := m.vp.View()
	if strings.Contains(body, "new-tail") && !strings.Contains(body, "row-00") {
		t.Fatalf("viewport jumped to tail:\n%s", body)
	}
}

func TestFormModelsDoNotCoverSave(t *testing.T) {
	m := testModel(t)
	var models []string
	for i := 0; i < 40; i++ {
		models = append(models, fmt.Sprintf("mod-%02d", i))
	}
	m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: models})
	view := m.View()
	y, row, ok := findPlainRow(view, "save")
	if !ok {
		t.Fatalf("save missing:\n%s", stripANSI(view))
	}
	x := strings.Index(row, "save")
	got, ok := m.hits.At(x, y)
	if !ok || got.Kind != KindFormSave {
		t.Fatalf("save hit = %#v ok=%v", got, ok)
	}
	for _, hit := range m.hits.OfKind(KindFormListItem) {
		if hit.Rect.Y == y {
			t.Fatalf("model row painted on save y=%d: %#v", y, hit)
		}
	}
}

func TestNavbarControls(t *testing.T) {
	m := testModel(t)
	plain := stripANSI(m.View())
	label := untitledSession
	ws := workspaceName(m.h)
	for _, want := range []string{label, ws, glyphPlus, glyphClock, glyphGear, glyphQuit} {
		if !strings.Contains(plain, want) {
			t.Fatalf("navbar missing %q:\n%s", want, plain)
		}
	}
	nav := strings.Split(plain, "\n")[0]
	if strings.Index(nav, ws) < 0 || strings.Index(nav, label) < 0 || strings.Index(nav, ws) > strings.Index(nav, label) {
		t.Fatalf("workspace should be left of session title: %q", nav)
	}
	si, gi, pi := strings.Index(nav, label), strings.Index(nav, "siggy"), strings.Index(nav, glyphPlus)
	if gi < 0 || si < 0 || pi < 0 || !(si < gi && gi < pi) {
		t.Fatalf("siggy should sit between session and icons: %q", nav)
	}
	mid := m.width / 2
	for _, k := range []Kind{KindSidebarNew, KindNavClock, KindNavGear, KindNavQuit} {
		tgot, ok := firstKind(&m, k)
		if !ok || tgot.Rect.X < mid {
			t.Fatalf("nav icon %v not on the right half: %#v", k, tgot)
		}
	}
	before := len(m.sessions)
	plus, ok := firstKind(&m, KindSidebarNew)
	if !ok {
		t.Fatal("missing +")
	}
	next, _ := m.activate(plus)
	nm := next.(model)
	if len(nm.sessions) != before {
		t.Fatalf("empty + should not list a session: %#v", nm.sessions)
	}
	if nm.page != pageSession {
		t.Fatalf("page = %d", nm.page)
	}

	quit, ok := firstKind(&nm, KindNavQuit)
	if !ok {
		t.Fatal("missing quit")
	}
	_, cmd := nm.activate(quit)
	if cmd == nil {
		t.Fatal("quit returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.Quit")
	}
}

func TestNavWorkspacePicker(t *testing.T) {
	m := testModel(t)
	root := m.h.Workspace.Root
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, ok := firstKind(&m, KindNavWorkspace)
	if !ok {
		t.Fatal("missing workspace hit")
	}
	next, _ := m.activate(ws)
	m = next.(model)
	if m.float != floatWorkspace {
		t.Fatalf("float = %d", m.float)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "workspace") || !strings.Contains(plain, "use this folder") {
		t.Fatalf("picker missing chrome:\n%s", plain)
	}
	if !strings.Contains(plain, "sub") {
		t.Fatalf("missing sub dir:\n%s", plain)
	}
	if strings.Contains(plain, "file.txt") || strings.Contains(plain, ".hidden") {
		t.Fatalf("listed non-dir or dot dir:\n%s", plain)
	}
	if _, ok := firstKind(&m, KindWorkspaceUp); ok {
		t.Fatal(".. should be hidden at root")
	}
	dir, ok := firstKind(&m, KindWorkspaceDir)
	if !ok {
		t.Fatal("missing dir")
	}
	next, _ = m.activate(dir)
	m = next.(model)
	if m.wsBrowse != sub {
		t.Fatalf("browse = %s want %s", m.wsBrowse, sub)
	}
	_ = m.View()
	up, ok := firstKind(&m, KindWorkspaceUp)
	if !ok {
		t.Fatal("missing ..")
	}
	next, _ = m.activate(up)
	m = next.(model)
	if m.wsBrowse != root {
		t.Fatalf("browse after up = %s", m.wsBrowse)
	}
	_ = m.View()
	if _, ok := firstKind(&m, KindWorkspaceUp); ok {
		t.Fatal(".. listed a parent of root")
	}
	next, _ = m.activate(dir)
	m = next.(model)
	_ = m.View()
	use, ok := firstKind(&m, KindWorkspaceUse)
	if !ok {
		t.Fatal("missing use")
	}
	next, _ = m.activate(use)
	m = next.(model)
	if m.float != floatNone {
		t.Fatal("picker still open")
	}
	if m.h.Workspace.Root != sub {
		t.Fatalf("workspace = %s", m.h.Workspace.Root)
	}
	if m.wsRoot != root {
		t.Fatalf("wsRoot changed to %s", m.wsRoot)
	}
	if m.cfg.Workspace != root {
		t.Fatalf("cfg.Workspace persisted: %s", m.cfg.Workspace)
	}
	nav := strings.Split(stripANSI(m.View()), "\n")[0]
	if !strings.Contains(nav, "sub") {
		t.Fatalf("navbar missing sub: %q", nav)
	}
}

func TestNavTitleReturnsToSession(t *testing.T) {
	m := testModel(t)
	title, ok := firstKind(&m, KindNavTitle)
	if !ok {
		t.Fatal("missing title hit")
	}
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	if m.page != pageSettings {
		t.Fatalf("page = %d", m.page)
	}
	next, _ = m.activate(title)
	m = next.(model)
	if m.page != pageSession {
		t.Fatalf("page = %d", m.page)
	}
	if m.float != floatNone {
		t.Fatal("float still open")
	}
}

func TestClockMenuDeletes(t *testing.T) {
	m := testModel(t)
	extra, err := harness.NewSession(m.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := extra.Append(harness.Record{Type: "user", Text: "listed"}); err != nil {
		t.Fatal(err)
	}
	extra.Close()
	m.reloadSessions()
	clock, ok := firstKind(&m, KindNavClock)
	if !ok {
		t.Fatal("missing clock")
	}
	next, _ := m.activate(clock)
	nm := next.(model)
	if nm.float != floatSessions {
		t.Fatalf("float = %d", nm.float)
	}
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, untitledSession) {
		t.Fatalf("clock menu missing %s:\n%s", untitledSession, plain)
	}
	if strings.Contains(plain, extra.ID) {
		t.Fatalf("clock menu leaked id %s:\n%s", extra.ID, plain)
	}
	if !strings.Contains(plain, "delete all") {
		t.Fatalf("clock menu missing delete all:\n%s", plain)
	}
	if nm.reg.modal.Y != navRows {
		t.Fatalf("sessions menu y=%d want %d", nm.reg.modal.Y, navRows)
	}
	clockR := nm.reg.navClock
	if !nm.reg.modal.Contains(clockR.X, nm.reg.modal.Y) && !nm.reg.modal.Contains(clockR.X+clockR.W-1, nm.reg.modal.Y) {
		t.Fatalf("sessions menu %#v does not overlap clock %#v", nm.reg.modal, clockR)
	}
	next, _ = nm.activate(clock)
	closed := next.(model)
	if closed.float != floatNone {
		t.Fatal("second clock click should close the menu")
	}
	next, _ = closed.activate(clock)
	nm = next.(model)
	idx := -1
	for i, id := range nm.sessions {
		if id == extra.ID {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("extra session not listed")
	}
	next, _ = nm.activate(Target{Kind: KindSidebarDelete, Index: idx})
	nm = next.(model)
	if containsStr(nm.sessions, extra.ID) {
		t.Fatalf("session still listed: %#v", nm.sessions)
	}
	if nm.float != floatNone {
		t.Fatal("menu still open after delete")
	}
	if harness.SessionExists(nm.home, extra.ID) {
		t.Fatalf("session still exists")
	}
}

func TestGearOpensSettings(t *testing.T) {
	m := testModel(t)
	m.err = `llm http 401: { "error": {`
	m.lines = append(m.lines, line{kind: "sys", text: "resumed 20260818T000005Z"})
	gear, ok := firstKind(&m, KindNavGear)
	if !ok {
		t.Fatal("missing gear")
	}
	next, _ := m.activate(gear)
	nm := next.(model)
	if nm.page != pageSettings {
		t.Fatalf("page = %d", nm.page)
	}
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, "Providers") {
		t.Fatalf("settings sidebar missing Providers:\n%s", plain)
	}
	if !strings.Contains(plain, "Version") {
		t.Fatalf("settings sidebar missing Version:\n%s", plain)
	}
	for _, leak := range []string{"session ready", "resumed", "llm http"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("settings page leaked %q:\n%s", leak, plain)
		}
	}
}

func TestVersionTabShowsValue(t *testing.T) {
	m := testModel(t)
	next, _ := m.activate(Target{Kind: KindNavGear})
	m = next.(model)
	next, _ = m.activate(Target{Kind: KindSidebarVersion})
	m = next.(model)
	if m.page != pageSettings || m.tab != settingsTabVersion {
		t.Fatalf("page=%d tab=%d", m.page, m.tab)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, version.Value) {
		t.Fatalf("missing version %q:\n%s", version.Value, plain)
	}
	for _, leak := range []string{"new provider", " edit ", "openai"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("version tab leaked %q:\n%s", leak, plain)
		}
	}
}

func TestPageSwitchHidesSessionCopy(t *testing.T) {
	m := testModel(t)
	m.err = `llm http 401: { "error": {`
	m.lines = append(m.lines, line{kind: "sys", text: "resumed 20260818T000005Z"})
	m.leaveSessionPage()
	m.page = pageSettings
	plain := stripANSI(m.View())
	for _, leak := range []string{"session ready", "resumed", "llm http"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("settings page leaked %q:\n%s", leak, plain)
		}
	}
	m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: []string{""}})
	plain = stripANSI(m.View())
	for _, leak := range []string{"session ready", "resumed", "llm http"} {
		if strings.Contains(plain, leak) {
			t.Fatalf("form page leaked %q:\n%s", leak, plain)
		}
	}
}

func TestDeleteSessionRemovesFile(t *testing.T) {
	m := testModel(t)
	extra, err := harness.NewSession(m.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := extra.Append(harness.Record{Type: "user", Text: "listed"}); err != nil {
		t.Fatal(err)
	}
	extra.Close()
	m.reloadSessions()
	_ = m.View()
	idx := -1
	for i, id := range m.sessions {
		if id == extra.ID {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("extra session not listed")
	}
	next, _ := m.activate(Target{Kind: KindSidebarDelete, Index: idx})
	nm := next.(model)
	if containsStr(nm.sessions, extra.ID) {
		t.Fatalf("session still listed: %#v", nm.sessions)
	}
	if harness.SessionExists(nm.home, extra.ID) {
		t.Fatalf("session still exists")
	}
}

func TestDeleteAllLeavesOneSession(t *testing.T) {
	m := testModel(t)
	extra, err := harness.NewSession(m.home)
	if err != nil {
		t.Fatal(err)
	}
	if err := extra.Append(harness.Record{Type: "user", Text: "listed"}); err != nil {
		t.Fatal(err)
	}
	extra.Close()
	m.reloadSessions()
	next, _ := m.activate(Target{Kind: KindSidebarDeleteAll})
	nm := next.(model)
	if len(nm.sessions) != 0 {
		t.Fatalf("sessions = %#v", nm.sessions)
	}
	if nm.h.Session == nil {
		t.Fatal("expected a fresh active session")
	}
}

func TestFormDeleteModel(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: []string{"keep", "drop"}})
	_ = m.View()
	next, _ := m.activate(Target{Kind: KindFormDeleteModel, Index: 1})
	nm := next.(model)
	if len(nm.form.models) != 1 || nm.form.models[0] != "keep" {
		t.Fatalf("models = %#v", nm.form.models)
	}
	view := nm.View()
	y, row, ok := findPlainRow(view, "save")
	if !ok {
		t.Fatal("save missing after delete")
	}
	got, ok := nm.hits.At(strings.Index(row, "save"), y)
	if !ok || got.Kind != KindFormSave {
		t.Fatalf("save hit = %#v", got)
	}
}

func TestUsageCircleAndMention(t *testing.T) {
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "note.md"), []byte("hello file"), 0o644); err != nil {
		t.Fatal(err)
	}
	next, _ := m.onEvent(loop.Event{Kind: loop.KindUsage, PromptTokens: 64000})
	nm := next.(model)
	if nm.tokensUsed != 64000 {
		t.Fatalf("tokensUsed = %d", nm.tokensUsed)
	}
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, usageBadge(64000, nm.contextWindow())) {
		t.Fatalf("usage glyph missing:\n%s", plain)
	}
	if strings.Contains(plain, "◑") || strings.Contains(plain, "◕") {
		t.Fatalf("old moon glyph still present:\n%s", plain)
	}

	nm.ta.SetValue("@note")
	nm.syncMentions()
	_ = nm.View()
	if nm.float != floatMentions {
		t.Fatalf("float = %d", nm.float)
	}
	hit, ok := firstKind(&nm, KindMention)
	if !ok {
		t.Fatalf("mention list empty: %#v", nm.mentions)
	}
	next, _ = nm.activate(hit)
	nm = next.(model)
	if !strings.Contains(nm.ta.Value(), "@note.md") {
		t.Fatalf("insert = %q mentions=%#v", nm.ta.Value(), nm.mentions)
	}
	expanded := expandMentions(nm.h, "@note.md")
	if !strings.Contains(expanded, "hello file") {
		t.Fatalf("expand = %q", expanded)
	}
}

func TestExpandMentionsSkipsPDF(t *testing.T) {
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "paper.pdf"), []byte("%PDF-1.4\nbinary"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := expandMentions(m.h, "@paper.pdf")
	if strings.Contains(got, "%PDF") || strings.Contains(got, "<file") {
		t.Fatalf("inlined pdf: %q", got)
	}
}

func TestListArrowEnter(t *testing.T) {
	m := testModel(t)
	m.openFloat(floatMode)
	_ = m.View()
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyDown})
	nm := next.(model)
	if nm.palIdx != 1 {
		t.Fatalf("palIdx = %d", nm.palIdx)
	}
	next, _ = nm.onKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm = next.(model)
	if nm.g.Engine.Harness.Mode != harness.ModePlan {
		t.Fatalf("mode = %s", nm.g.Engine.Harness.Mode)
	}
	if nm.float != floatNone {
		t.Fatal("menu still open")
	}
	next, _ = nm.onKey(tea.KeyMsg{Type: tea.KeyEsc})
	nm = next.(model)
	if nm.float != floatNone {
		t.Fatal("esc should keep float closed")
	}
}

func TestRightClickDoesNotOpenMenu(t *testing.T) {
	m := testModel(t)
	prompt, ok := firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	next, cmd := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonRight, Action: tea.MouseActionPress, X: prompt.Rect.X, Y: prompt.Rect.Y})
	if cmd != nil {
		t.Fatal("right-click should not emit a command")
	}
	nm := next.(model)
	if nm.float != floatNone {
		t.Fatalf("right-click opened float %d", nm.float)
	}
}

func TestSelectionCopyCutPaste(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("abcdef")
	prompt, ok := firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	x0, y := prompt.Rect.X, prompt.Rect.Y
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: x0 + 1, Y: y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: x0 + 4, Y: y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: x0 + 4, Y: y})
	m = next.(model)
	if m.selectedText() != "bcd" {
		t.Fatalf("selected %q anchor=%d caret=%d", m.selectedText(), m.selAnchor, m.selCaret)
	}
	next, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if isQuitCmd(cmd) {
		t.Fatal("ctrl+c with selection should not quit")
	}
	if m.editClip != "bcd" {
		t.Fatalf("copy clip = %q", m.editClip)
	}
	m.selAnchor, m.selCaret = 1, 4
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = next.(model)
	if m.ta.Value() != "aef" {
		t.Fatalf("cut = %q", m.ta.Value())
	}
	m.ta.SetValue("abcdef")
	m.selAnchor, m.selCaret = 1, 4
	m.editClip = "XY"
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = next.(model)
	if m.ta.Value() != "aXYef" {
		t.Fatalf("paste over sel = %q", m.ta.Value())
	}
}

func TestNoSelectionCopyCutNoop(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("keep")
	m.selCaret, m.selAnchor = 2, 2
	m.editClip = "old"
	if cmd := m.editCopy(); cmd != nil {
		t.Fatal("copy with no selection should be a no-op")
	}
	if m.editClip != "old" {
		t.Fatalf("copy with no sel changed clip %q", m.editClip)
	}
	_ = m.editCut()
	if m.ta.Value() != "keep" || m.editClip != "old" {
		t.Fatalf("cut no-sel value=%q clip=%q", m.ta.Value(), m.editClip)
	}
	m.editPaste("XY")
	if m.ta.Value() != "keXYep" {
		t.Fatalf("paste insert = %q", m.ta.Value())
	}
}

func TestSelectAllAndDoubleClick(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("hello")
	prompt, ok := firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = next.(model)
	if m.selectedText() != "hello" {
		t.Fatalf("select all = %q", m.selectedText())
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "hello") {
		t.Fatalf("selected text not visible:\n%s", plain)
	}
	next, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if isQuitCmd(cmd) {
		t.Fatal("ctrl+c after select-all should not quit")
	}
	if m.editClip != "hello" {
		t.Fatalf("copy all = %q", m.editClip)
	}

	m = testModel(t)
	m.ta.SetValue("hello")
	prompt, ok = firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	m.lastClickAt = time.Now().Add(-50 * time.Millisecond)
	m.lastClick = prompt
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	if m.selectedText() != "hello" {
		t.Fatalf("double click sel = %q", m.selectedText())
	}

	m = testModel(t)
	m.ta.SetValue("hello")
	prompt, _ = firstKind(&m, KindPrompt)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	m.lastClickAt = time.Now().Add(-300 * time.Millisecond)
	m.lastClick = prompt
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X + 2, Y: prompt.Rect.Y})
	m = next.(model)
	if m.hasSel() {
		t.Fatalf("slow second click should not select all, sel=%q", m.selectedText())
	}
	if m.selCaret != 2 {
		t.Fatalf("caret = %d", m.selCaret)
	}
}

func TestClickPlacesCursor(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("abcdef")
	prompt, ok := firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X + 1, Y: prompt.Rect.Y})
	m = next.(model)
	if m.selCaret != 1 || m.hasSel() {
		t.Fatalf("in front of second: caret=%d sel=%v", m.selCaret, m.hasSel())
	}
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: prompt.Rect.X + 1, Y: prompt.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X + 6, Y: prompt.Rect.Y})
	m = next.(model)
	if m.selCaret != 6 {
		t.Fatalf("behind last: caret=%d", m.selCaret)
	}
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: prompt.Rect.X + 6, Y: prompt.Rect.Y})
	m = next.(model)

	m.openForm(config.Provider{Name: "abcdef", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	_ = m.View()
	name, ok := firstKind(&m, KindFormField)
	if !ok || name.Index != 0 {
		t.Fatalf("name field %#v ok=%v", name, ok)
	}
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: name.Rect.X + 1, Y: name.Rect.Y})
	m = next.(model)
	if m.selCaret != 1 {
		t.Fatalf("form in front of second: caret=%d field=%d", m.selCaret, m.form.field)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "abcdef") {
		t.Fatalf("form value missing:\n%s", plain)
	}
}

func TestFormSelectionCopyPaste(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "abcdef", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.selAnchor, m.selCaret = 1, 4
	next, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if isQuitCmd(cmd) {
		t.Fatal("form ctrl+c with selection should not quit")
	}
	if m.editClip != "bcd" {
		t.Fatalf("form copy = %q name=%q", m.editClip, m.form.name)
	}
	m.selAnchor, m.selCaret = 1, 4
	m.editClip = "XY"
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = next.(model)
	if m.form.name != "aXYef" {
		t.Fatalf("form paste = %q", m.form.name)
	}
}

func TestProtocolRightClickNoEditMenu(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	_ = m.View()
	var proto Target
	for _, hit := range m.hits.OfKind(KindFormField) {
		if hit.Index >= 10 {
			proto = hit
			break
		}
	}
	if proto.Kind == KindNone {
		t.Fatal("missing protocol row")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonRight, Action: tea.MouseActionPress, X: proto.Rect.X, Y: proto.Rect.Y})
	nm := next.(model)
	if nm.float != floatNone {
		t.Fatalf("protocol right-click opened float %d", nm.float)
	}
}

func TestEmptyApiKeyNoSGRLeak(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 2
	m.form.apiKey = ""
	m.selCaret, m.selAnchor = 0, 0
	m.cursorOn = true
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "1;30;47") || strings.Contains(plain, "[0m") {
		t.Fatalf("visible SGR in empty api key:\n%s", plain)
	}
	line := m.paintEditLine(nil, 0, 60)
	if n := strings.Count(line, stSel.Render(" ")); n != 1 {
		t.Fatalf("api key caret stretched to %d cells", n)
	}
}

func TestInsertTextStripsANSI(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 2
	m.form.apiKey = ""
	m.selCaret, m.selAnchor = 0, 0
	m.insertText("\x1b[1;30;47msecret\x1b[0m")
	if m.form.apiKey != "secret" {
		t.Fatalf("apiKey = %q", m.form.apiKey)
	}
}

func TestFormBracketedPaste(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 2
	m.form.apiKey = ""
	m.selCaret, m.selAnchor = 0, 0
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("sk-test")})
	nm := next.(model)
	if nm.form.apiKey != "sk-test" {
		t.Fatalf("pasted apiKey = %q", nm.form.apiKey)
	}
}

func TestEmptyClipPasteNoGreyFill(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 2
	m.form.apiKey = "kept"
	m.selCaret = 4
	m.collapseSel()
	m.editClip = ""
	m.cursorOn = true
	m.editPaste("")
	if m.form.apiKey != "kept" {
		t.Fatalf("empty clip mutated key %q", m.form.apiKey)
	}
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "1;30;47") || strings.Contains(plain, "[0m") {
		t.Fatalf("empty paste painted SGR:\n%s", plain)
	}
	if !strings.Contains(plain, "kept") {
		t.Fatalf("value hidden after empty paste:\n%s", plain)
	}
}

func TestRightClickEmptySpaceSwallowed(t *testing.T) {
	m := testModel(t)
	next, cmd := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonRight, Action: tea.MouseActionPress, X: 0, Y: 0})
	if cmd != nil {
		t.Fatal("right-click miss should not emit a command")
	}
	nm := next.(model)
	if nm.float != floatNone {
		t.Fatalf("right-click miss opened float %d", nm.float)
	}
}

func TestCtrlCNoSelectionQuits(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("keep")
	m.selCaret, m.selAnchor = 2, 2
	_, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatal("ctrl+c with no selection should quit")
	}
}

func TestApprovalTitleOmitsDuplicateRisk(t *testing.T) {
	m := testModel(t)
	m.approval = &harness.ApprovalRequest{Tool: "shell", Risk: "shell", Summary: "echo hi", Reply: make(chan harness.Decision, 1)}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "approve shell") {
		t.Fatalf("missing title: %q", plain)
	}
	if strings.Contains(plain, "approve shell  shell") || strings.Contains(plain, "approve shell shell") {
		t.Fatalf("duplicate risk in title: %q", plain)
	}
}

func TestViewLinesFitWidth(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	m.cursorOn = false
	view := m.View()
	for i, ln := range strings.Split(view, "\n") {
		if w := lipgloss.Width(ln); w > m.width {
			t.Fatalf("line %d width %d > %d: %q", i, w, m.width, ln)
		}
	}
}

func TestFormatToolCardShellCommand(t *testing.T) {
	got := formatToolCard("shell", `{"command":"ls -l"}`)
	if got != "shell  ls -l" {
		t.Fatalf("got %q", got)
	}
}

func TestReplyMetaFooter(t *testing.T) {
	m := testModel(t)
	next, _ := m.onEvent(loop.Event{Kind: loop.KindText, Text: "hello there"})
	m = next.(model)
	if last := m.lines[len(m.lines)-1]; last.kind != "asst-live" || last.model != "" {
		t.Fatalf("footer while streaming: %#v", last)
	}
	next, _ = m.onEvent(loop.Event{Kind: loop.KindUsage, CompletionTokens: 10, TotalTokens: 15})
	m = next.(model)
	next, _ = m.onEvent(loop.Event{Kind: loop.KindDone})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if last.kind != "asst-live" {
		t.Fatalf("kind = %s", last.kind)
	}
	if last.model != "env / gpt-4.1" {
		t.Fatalf("model = %q", last.model)
	}
	if last.toks != 10 {
		t.Fatalf("toks = %d", last.toks)
	}
	got := stripANSI(m.renderLine(last, 40))
	if !strings.Contains(got, "env / gpt-4.1") || !strings.Contains(got, "tok/s") {
		t.Fatalf("footer = %q", got)
	}
}

func TestToolEndDiffNotTruncated(t *testing.T) {
	m := testModel(t)
	var b strings.Builder
	b.WriteString("edited foo.go\n")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, "- %4d | old-line-content-here\n", i+1)
		fmt.Fprintf(&b, "+ %4d | new-line-content-here\n", i+1)
	}
	text := b.String()
	if len(text) <= 240 {
		t.Fatalf("fixture too short: %d", len(text))
	}
	next, _ := m.onEvent(loop.Event{Kind: loop.KindToolEnd, Tool: "edit_file", Text: text})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if last.kind != "diff" {
		t.Fatalf("kind = %s text=%q", last.kind, last.text)
	}
	if len(last.text) <= 240 {
		t.Fatalf("truncated to %d", len(last.text))
	}
}

func TestComposerUpDownMovesCaret(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("ab\ncd")
	m.selCaret, m.selAnchor = 0, 0
	_ = m.View()
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.selCaret != 3 {
		t.Fatalf("down caret = %d", m.selCaret)
	}
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if m.selCaret != 0 {
		t.Fatalf("up caret = %d", m.selCaret)
	}
}

func TestFormUpFromURLFocusesName(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 1
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyUp})
	nm := next.(model)
	if nm.form.field != 0 {
		t.Fatalf("field = %d", nm.form.field)
	}
}

func TestComposerSpaceInsertsInPlace(t *testing.T) {
	m := testModel(t)
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(model)
	if m.ta.Value() != "a b" {
		t.Fatalf("got %q", m.ta.Value())
	}
	if m.selCaret != 3 {
		t.Fatalf("caret = %d", m.selCaret)
	}
}

func TestFormSpaceInsertsInPlace(t *testing.T) {
	m := testModel(t)
	m.openForm(config.Provider{Name: "p", URL: "http://x", Models: []string{"m"}, Protocols: []string{config.ProtocolOpenAI}})
	m.form.field = 0
	m.form.name = ""
	m.selCaret, m.selAnchor = 0, 0
	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(model)
	if m.form.name != "a b" {
		t.Fatalf("got %q", m.form.name)
	}
	if m.selCaret != 3 {
		t.Fatalf("caret = %d", m.selCaret)
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func firstKind(m *model, k Kind) (Target, bool) {
	if m.hits == nil {
		return Target{}, false
	}
	for _, t := range m.hits.targets {
		if t.Kind == k {
			return t, true
		}
	}
	return Target{}, false
}

func findPlainRow(view, needle string) (int, string, bool) {
	for i, line := range strings.Split(stripANSI(view), "\n") {
		if strings.Contains(line, needle) {
			return i, line, true
		}
	}
	return 0, "", false
}

func TestResumeShowsTranscript(t *testing.T) {
	m := testModel(t)
	if err := m.h.Session.Append(harness.Record{Type: "user", Text: "hello from user"}); err != nil {
		t.Fatal(err)
	}
	if err := m.h.Session.Append(harness.Record{Type: "assistant", Text: "hello from asst"}); err != nil {
		t.Fatal(err)
	}
	id := m.h.Session.ID
	m.openSession(id)
	var user, asst bool
	for _, ln := range m.lines {
		if ln.kind == "user" && strings.Contains(ln.text, "hello from user") {
			user = true
		}
		if (ln.kind == "asst-live" || ln.kind == "asst") && strings.Contains(ln.text, "hello from asst") {
			asst = true
		}
	}
	if !user || !asst {
		t.Fatalf("resume transcript missing turns: %#v", m.lines)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "hello from user") || !strings.Contains(plain, "hello from asst") {
		t.Fatalf("view missing restored turns:\n%s", plain)
	}
}

func TestNewLoadsExistingTranscript(t *testing.T) {
	m := testModel(t)
	if err := m.h.Session.Append(harness.Record{Type: "user", Text: "prior user"}); err != nil {
		t.Fatal(err)
	}
	if err := m.h.Session.Append(harness.Record{Type: "assistant", Text: "prior asst"}); err != nil {
		t.Fatal(err)
	}
	g := graph.FromSession(m.g.Engine.LLM, tools.Builtins(m.h, nil), m.h)
	nm := New(g, m.h, m.cfg)
	nm.width, nm.height = 80, 24
	nm.layout()
	nm.refresh()
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, "prior user") || !strings.Contains(plain, "prior asst") {
		t.Fatalf("New(--resume) empty transcript:\n%s", plain)
	}
	nm.leaveSessionPage()
	nm.page = pageSettings
	plain = stripANSI(nm.View())
	if strings.Contains(plain, "prior user") || strings.Contains(plain, "prior asst") {
		t.Fatalf("settings leaked transcript:\n%s", plain)
	}
}

func TestHelpListsNewCommands(t *testing.T) {
	m := testModel(t)
	next, _ := m.command("/help")
	nm := next.(model)
	text := nm.lines[len(nm.lines)-1].text
	for _, cmd := range []string{"/compress", "/export", "/restore", "/branch", "/rewind", "/remember"} {
		if !strings.Contains(text, cmd) {
			t.Fatalf("help missing %s: %s", cmd, text)
		}
	}
}

func TestExportCommandWritesFile(t *testing.T) {
	m := testModel(t)
	if err := m.h.Session.Append(harness.Record{Type: "user", Text: "exported-turn"}); err != nil {
		t.Fatal(err)
	}
	next, _ := m.command("/export jsonl")
	nm := next.(model)
	name := "siggy-export-" + nm.h.Session.ID + ".jsonl"
	raw, err := os.ReadFile(filepath.Join(nm.h.Workspace.Root, name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "exported-turn") {
		t.Fatalf("export = %s", raw)
	}
}

func TestUsageGaugeOpensCounts(t *testing.T) {
	m := testModel(t)
	m.tokensUsed = 6400
	_ = m.View()
	hit, ok := firstKind(&m, KindUsage)
	if !ok {
		t.Fatal("missing usage target")
	}
	next, _ := m.activate(hit)
	nm := next.(model)
	if nm.float != floatUsage {
		t.Fatalf("float = %d", nm.float)
	}
	plain := stripANSI(nm.View())
	wantCtx := fmt.Sprintf("context %d / %d", nm.usageUsed(), nm.contextWindow())
	if !strings.Contains(plain, wantCtx) {
		t.Fatalf("missing %q:\n%s", wantCtx, plain)
	}
	if !strings.Contains(plain, "billed 0 this session") {
		t.Fatalf("missing billed line:\n%s", plain)
	}
	nm.leaveSessionPage()
	nm.page = pageSettings
	plain = stripANSI(nm.View())
	if strings.Contains(plain, wantCtx) {
		t.Fatalf("settings leaked usage popup:\n%s", plain)
	}
}

func TestSessionTitleAfterQA(t *testing.T) {
	m := testModel(t)
	if err := m.h.Session.Append(harness.Record{Type: "user", Text: "schedule n jobs"}); err != nil {
		t.Fatal(err)
	}
	if err := m.h.Session.Append(harness.Record{Type: "assistant", Text: "assign greedily"}); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.onEvent(loop.Event{Kind: loop.KindDone})
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected title command")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	got := m.h.Session.Meta.Title
	if got == "" || got == m.h.Session.ID {
		t.Fatalf("title = %q", got)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, got) {
		t.Fatalf("navbar missing title %q:\n%s", got, plain)
	}
	if strings.Contains(plain, untitledSession) {
		t.Fatalf("still untitled:\n%s", plain)
	}
}

func TestUsageSplitsContextAndBilled(t *testing.T) {
	m := testModel(t)
	next, _ := m.onEvent(loop.Event{Kind: loop.KindUsage, PromptTokens: 639, CompletionTokens: 4983, TotalTokens: 5622})
	m = next.(model)
	if m.tokensUsed != 639 {
		t.Fatalf("context = %d", m.tokensUsed)
	}
	if m.billedTokens != 5622 {
		t.Fatalf("billed = %d", m.billedTokens)
	}
	next, _ = m.onEvent(loop.Event{Kind: loop.KindUsage, PromptTokens: 800, CompletionTokens: 200, TotalTokens: 1000})
	m = next.(model)
	if m.tokensUsed != 800 {
		t.Fatalf("context after second = %d", m.tokensUsed)
	}
	if m.billedTokens != 6622 {
		t.Fatalf("billed after second = %d", m.billedTokens)
	}
	m.float = floatUsage
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "context 800 / 128000") {
		t.Fatalf("popup missing context:\n%s", plain)
	}
	if !strings.Contains(plain, "billed 6622 this session") {
		t.Fatalf("popup missing billed:\n%s", plain)
	}
}

func TestUsageResumeRestoresBilled(t *testing.T) {
	m := testModel(t)
	if err := m.h.Session.Append(harness.Record{Type: "user", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := m.h.Session.Append(harness.Record{
		Type:             "usage",
		PromptTokens:     639,
		CompletionTokens: 4983,
		TotalTokens:      5622,
	}); err != nil {
		t.Fatal(err)
	}
	m.loadTranscript("resumed")
	if m.tokensUsed != 639 {
		t.Fatalf("context = %d", m.tokensUsed)
	}
	if m.billedTokens != 5622 {
		t.Fatalf("billed = %d", m.billedTokens)
	}
	m.float = floatUsage
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "context 639 / 128000") || !strings.Contains(plain, "billed 5622 this session") {
		t.Fatalf("popup =\n%s", plain)
	}
}

func TestUsageEstimatedFlag(t *testing.T) {
	m := testModel(t)
	next, _ := m.onEvent(loop.Event{Kind: loop.KindUsage, PromptTokens: 100, TotalTokens: 100, Estimated: true})
	m = next.(model)
	if !m.billedEst || m.billedTokens != 100 {
		t.Fatalf("est=%v billed=%d", m.billedEst, m.billedTokens)
	}
	m.float = floatUsage
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "est") {
		t.Fatalf("missing est marker:\n%s", plain)
	}
}
