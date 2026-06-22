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

	// Skip the banner -> mode picker.
	m = step(m, bannerDoneMsg{})
	modes := m.View()
	if !strings.Contains(modes, "choose a channel") || !strings.Contains(modes, "ORACLE · doctrine") {
		t.Fatalf("mode screen wrong:\n%s", modes)
	}

	// Doctrine: enter on the first mode goes straight to chat, no skill.
	md := step(m, key("enter"))
	if md.screen != screenChat || md.mode != ModeDoctrine {
		t.Fatalf("doctrine should jump to chat; screen=%d mode=%q", md.screen, md.mode)
	}

	// Generic: third mode -> category menu -> skills.
	g := step(step(step(m, key("down")), key("down")), key("enter"))
	if g.screen != screenCats || g.mode != ModeGeneric {
		t.Fatalf("generic should open categories; screen=%d mode=%q", g.screen, g.mode)
	}
	cats := g.View()
	if !strings.Contains(cats, "select a category") || !strings.Contains(cats, "THINKING FRAMEWORKS") {
		t.Fatalf("category screen wrong:\n%s", cats)
	}
	g = step(g, key("enter")) // open first category
	if g.screen != screenSkills {
		t.Fatalf("expected skills screen, got %d", g.screen)
	}
	if sv := g.View(); !strings.Contains(sv, "inversion") || !strings.Contains(sv, "load module") {
		t.Fatalf("skills screen wrong:\n%s", sv)
	}
	if g = step(g, key("esc")); g.screen != screenCats {
		t.Fatalf("generic esc should return to categories, got %d", g.screen)
	}

	// Voice: second mode jumps straight to the After— skills, esc returns to modes.
	v := step(step(m, key("down")), key("enter"))
	if v.screen != screenSkills || v.mode != ModeVoice || v.activeCat != "After —" {
		t.Fatalf("voice should open After— skills; screen=%d mode=%q cat=%q", v.screen, v.mode, v.activeCat)
	}
	if v = step(v, key("esc")); v.screen != screenModes {
		t.Fatalf("voice esc should return to modes, got %d", v.screen)
	}
}

func TestSystemForDoctrine(t *testing.T) {
	s := testStore()
	sys, err := s.SystemFor(nil, ModeDoctrine, "")
	if err != nil {
		t.Fatalf("doctrine system: %v", err)
	}
	if !strings.Contains(sys, "THE ORACLE of the Church of Conceptual Art") {
		t.Fatal("doctrine prompt missing Oracle identity")
	}
}

func TestChatRenderAndGuards(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	m := newModel(r, testStore(), NewOracle("", ""), NewLimiter(99, time.Minute, 999), "1.2.3.4")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	// Simulate a loaded generic module + a finished exchange, then check rendering.
	m.screen = screenChat
	m.mode = ModeGeneric
	m.skillName = "after-nietzsche"
	m.system = "SYS"
	m.msgs = []Msg{
		{Role: "user", Content: "what is value?"},
		{Role: "assistant", Content: "Trace the value to its origin."},
	}
	m.renderTranscript()
	v := m.View()
	for _, want := range []string{"open channel", "after-nietzsche", "what is value?"} {
		if !strings.Contains(v, want) {
			t.Fatalf("chat view missing %q:\n%s", want, v)
		}
	}

	// Refusal/error replies land as assistant turns, not crashes.
	m2 := step(m, replyMsg{err: ErrRefused})
	if last := m2.msgs[len(m2.msgs)-1]; last.Content != "[ request refused ]" {
		t.Fatalf("refusal not rendered as assistant turn: %+v", last)
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
