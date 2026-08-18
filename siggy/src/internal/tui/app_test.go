package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
	m.ta.SetCursor(1)
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
	if !strings.Contains(plain, usageGlyph(m.usageUsed(), contextLimit)) {
		t.Fatalf("usage circle missing:\n%s", plain)
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
	sid := m.h.Session.ID
	ws := workspaceName(m.h)
	for _, want := range []string{sid, ws, glyphPlus, glyphClock, glyphGear, glyphQuit} {
		if !strings.Contains(plain, want) {
			t.Fatalf("navbar missing %q:\n%s", want, plain)
		}
	}
	nav := strings.Split(plain, "\n")[0]
	if strings.Index(nav, ws) < 0 || strings.Index(nav, sid) < 0 || strings.Index(nav, ws) > strings.Index(nav, sid) {
		t.Fatalf("workspace should be left of session title: %q", nav)
	}
	si, gi, pi := strings.Index(nav, sid), strings.Index(nav, "siggy"), strings.Index(nav, glyphPlus)
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
	if len(nm.sessions) != before+1 {
		t.Fatalf("sessions after + = %#v", nm.sessions)
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

func TestClockMenuDeletes(t *testing.T) {
	m := testModel(t)
	extra, err := harness.NewSession(m.home)
	if err != nil {
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
	if !strings.Contains(plain, extra.ID) {
		t.Fatalf("clock menu missing session %s:\n%s", extra.ID, plain)
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
	if _, err := os.Stat(filepath.Join(harness.SessionsDir(nm.home), extra.ID+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("session file still exists: %v", err)
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
	for _, leak := range []string{" new ", " edit ", "openai"} {
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
	if _, err := os.Stat(filepath.Join(harness.SessionsDir(nm.home), extra.ID+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("session file still exists: %v", err)
	}
}

func TestDeleteAllLeavesOneSession(t *testing.T) {
	m := testModel(t)
	extra, err := harness.NewSession(m.home)
	if err != nil {
		t.Fatal(err)
	}
	extra.Close()
	m.reloadSessions()
	next, _ := m.activate(Target{Kind: KindSidebarDeleteAll})
	nm := next.(model)
	if len(nm.sessions) != 1 {
		t.Fatalf("sessions = %#v", nm.sessions)
	}
	if nm.h.Session == nil || nm.h.Session.ID != nm.sessions[0] {
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
	next, _ := m.onEvent(loop.Event{Kind: loop.KindUsage, TotalTokens: 15})
	nm := next.(model)
	if nm.tokensUsed != 15 {
		t.Fatalf("tokensUsed = %d", nm.tokensUsed)
	}
	plain := stripANSI(nm.View())
	if !strings.Contains(plain, usageGlyph(15, contextLimit)) {
		t.Fatalf("usage glyph missing:\n%s", plain)
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
