package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

const sampleIndex = `# CoCA Skills

## Thinking Frameworks
- [inversion](inversion/SKILL.md): Ask how it fails, then reverse each failure into a safeguard.
- [first-principles](first-principles/SKILL.md): Break to bedrock facts and rebuild.

## After —
- [after-nietzsche](after-nietzsche/SKILL.md): Ask where a value came from and whom it serves.

## Distillations
- [Two-Way Doors](distillations/two-way-doors.md): Reversible vs irreversible decisions.

## Optional
- [Home](https://example.com/for-machines.html): the index page.
`

func testStore() *Skills {
	s := NewSkills("http://example.invalid")
	idx, cats := parseIndex(sampleIndex)
	s.index, s.cats = idx, cats
	s.byName = map[string]Skill{}
	for _, sk := range idx {
		s.byName[sk.Name] = sk
	}
	return s
}

func step(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestIndexParsing(t *testing.T) {
	s := testStore()
	if got := s.Count(); got != 3 {
		t.Fatalf("expected 3 skills (distillations/optional skipped), got %d", got)
	}
	if cats := s.Categories(); len(cats) != 2 || cats[0] != "Thinking Frameworks" || cats[1] != "After —" {
		t.Fatalf("unexpected categories: %v", cats)
	}
	if s.has("two-way-doors") {
		t.Fatal("distillation leaked into the skill allowlist")
	}
	if !s.has("inversion") {
		t.Fatal("inversion missing from allowlist")
	}
}

func TestMenuFlow(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	m := newModel(r, testStore(), NewOracle("", ""), NewLimiter(99, time.Minute, 999), "1.2.3.4")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if !strings.Contains(m.View(), "establishing link") {
		t.Fatalf("banner missing connect sequence:\n%s", m.View())
	}

	// Skip the banner -> engine picker.
	m = step(m, bannerDoneMsg{})
	eng := m.View()
	if !strings.Contains(eng, "choose a channel") || !strings.Contains(eng, "THE ORACLE") {
		t.Fatalf("engine screen wrong:\n%s", eng)
	}

	// Oracle -> voice pick -> raw -> chat (no voice).
	o := step(m, key("enter"))
	if o.screen != screenPick || o.picking != "voice" {
		t.Fatalf("oracle should open voice pick; screen=%d picking=%q", o.screen, o.picking)
	}
	if !strings.Contains(o.View(), "CHOOSE A VOICE") {
		t.Fatalf("voice pick header missing:\n%s", o.View())
	}
	o = step(o, key("enter")) // raw (index 0)
	if o.screen != screenChat || o.engine != EngineOracle || o.voice != "" {
		t.Fatalf("oracle raw should chat; screen=%d engine=%q voice=%q", o.screen, o.engine, o.voice)
	}

	// Open -> skill pick -> inversion -> voice pick -> no voice -> chat.
	g := step(step(m, key("down")), key("enter"))
	if g.screen != screenPick || g.picking != "skill" || g.engine != EngineOpen {
		t.Fatalf("open should open skill pick; screen=%d picking=%q", g.screen, g.picking)
	}
	if !strings.Contains(g.View(), "CHOOSE A SKILL") {
		t.Fatalf("skill pick header missing:\n%s", g.View())
	}
	g = step(g, key("down"))  // index 1 = inversion
	g = step(g, key("enter")) // choose skill -> voice pick
	if g.picking != "voice" || g.skill != "inversion" {
		t.Fatalf("after skill: picking=%q skill=%q", g.picking, g.skill)
	}
	g = step(g, key("enter")) // no voice -> connect
	if g.screen != screenChat || g.skill != "inversion" || g.voice != "" {
		t.Fatalf("open+skill should chat; screen=%d skill=%q voice=%q", g.screen, g.skill, g.voice)
	}

	// Open with neither skill nor voice -> flash, bounce back to skill pick.
	bad := step(step(m, key("down")), key("enter")) // open -> skill pick
	bad = step(bad, key("enter"))                   // no skill -> voice pick
	bad = step(bad, key("enter"))                   // no voice -> invalid
	if bad.screen != screenPick || bad.picking != "skill" || !strings.Contains(bad.flash, "pick a skill") {
		t.Fatalf("invalid open not handled; screen=%d picking=%q flash=%q", bad.screen, bad.picking, bad.flash)
	}

	// Install overlay: 'd' on a real skill shows commands; any key dismisses.
	ins := step(step(step(step(m, key("down")), key("enter")), key("down")), key("d"))
	if !ins.installing || ins.installName != "inversion" {
		t.Fatalf("install overlay not opened; installing=%v name=%q", ins.installing, ins.installName)
	}
	if !strings.Contains(ins.View(), "git clone") {
		t.Fatalf("install view missing commands:\n%s", ins.View())
	}
	if ins = step(ins, key("x")); ins.installing {
		t.Fatal("install overlay should dismiss on key")
	}
}

func TestSystemForCompositions(t *testing.T) {
	s := testStore()
	// oracle raw = doctrine only
	raw, err := s.SystemFor(nil, EngineOracle, "", "")
	if err != nil || !strings.Contains(raw, "THE ORACLE of the Church of Conceptual Art") {
		t.Fatalf("oracle raw wrong: %v\n%s", err, raw)
	}
	// open with neither -> error
	if _, err := s.SystemFor(nil, EngineOpen, "", ""); err == nil {
		t.Fatal("open with no skill/voice should error")
	}
}

func TestChatRenderAndGuards(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	m := newModel(r, testStore(), NewOracle("", ""), NewLimiter(99, time.Minute, 999), "1.2.3.4")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	// Simulate a loaded open-channel skill + a finished exchange.
	m.screen = screenChat
	m.engine = EngineOpen
	m.skill = "inversion"
	m.system = "SYS"
	m.msgs = []Msg{
		{Role: "user", Content: "what is value?"},
		{Role: "assistant", Content: "Trace the value to its origin."},
	}
	m.renderTranscript()
	v := m.View()
	for _, want := range []string{"open ·", "inversion", "what is value?"} {
		if !strings.Contains(v, want) {
			t.Fatalf("chat view missing %q:\n%s", want, v)
		}
	}

	// Refusal lands as an assistant turn (neutral copy for open channel).
	m2 := step(m, replyMsg{err: ErrRefused})
	if last := m2.msgs[len(m2.msgs)-1]; last.Content != "[ request refused ]" {
		t.Fatalf("refusal not rendered as assistant turn: %+v", last)
	}
}

func TestRendersWithoutWindowSize(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	m := newModel(r, testStore(), NewOracle("", ""), NewLimiter(99, time.Minute, 999), "ip")
	if m.View() == "" {
		t.Fatal("blank before any WindowSizeMsg (should render with the default size)")
	}
	// A 0×0 report (sent by some SSH clients) must not blank the UI.
	m = step(m, tea.WindowSizeMsg{Width: 0, Height: 0})
	if !strings.Contains(m.View(), "establishing link") {
		t.Fatalf("0×0 WindowSizeMsg blanked the banner:\n%q", m.View())
	}
}

func TestRateLimit(t *testing.T) {
	l := NewLimiter(2, time.Minute, 100)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("first hit should pass")
	}
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("second hit should pass")
	}
	if ok, reason := l.Allow("ip"); ok || reason == "" {
		t.Fatalf("third hit should be limited, got ok=%v reason=%q", ok, reason)
	}
}
