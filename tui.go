package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenBanner screen = iota
	screenModes
	screenCats
	screenSkills
	screenChat
)

type modeOption struct {
	key   Mode
	label string
	blurb string
}

var modeList = []modeOption{
	{ModeDoctrine, "ORACLE · doctrine", "the Church's own explanatory voice"},
	{ModeVoice, "ORACLE · after-voice", "the Oracle through an after— thinker"},
	{ModeGeneric, "OPEN CHANNEL · skill", "a generic model wearing one skill"},
}

const (
	maxInput = 2000
	maxTurns = 24 // 12 exchanges
)

// ---- styles (built per-session from the SSH renderer) ----

type styles struct {
	bright lipgloss.Style
	dim    lipgloss.Style
	accent lipgloss.Style
	muted  lipgloss.Style
	err    lipgloss.Style
	inv    lipgloss.Style // inverse / title chip
}

func newStyles(r *lipgloss.Renderer) styles {
	green := lipgloss.Color("#33ff66")
	dimg := lipgloss.Color("#2f9d5b")
	amber := lipgloss.Color("#ffcc33")
	gray := lipgloss.Color("#5f7065")
	red := lipgloss.Color("#ff5c5c")
	return styles{
		bright: r.NewStyle().Foreground(green),
		dim:    r.NewStyle().Foreground(dimg),
		accent: r.NewStyle().Foreground(amber),
		muted:  r.NewStyle().Foreground(gray),
		err:    r.NewStyle().Foreground(red),
		inv:    r.NewStyle().Foreground(lipgloss.Color("#04150a")).Background(green).Bold(true),
	}
}

// ---- list item ----

type skillItem struct{ name, desc string }

func (i skillItem) Title() string       { return i.name }
func (i skillItem) Description() string { return i.desc }
func (i skillItem) FilterValue() string { return i.name + " " + i.desc }

// ---- messages ----

type bannerDoneMsg struct{}
type skillLoadedMsg struct {
	name   string
	system string
	err    error
}
type replyMsg struct {
	text string
	err  error
}

// ---- model ----

type model struct {
	st     styles
	store  *Skills
	oracle *Oracle
	rl     *Limiter
	ip     string

	screen screen
	width  int
	height int

	mode    Mode
	modeIdx int

	cats   []string
	catIdx int

	list      list.Model
	activeCat string
	descW     int

	skillName string
	system    string
	loading   bool

	vp       viewport.Model
	ta       textarea.Model
	sp       spinner.Model
	msgs     []Msg
	thinking bool
	flash    string
}

func newModel(r *lipgloss.Renderer, store *Skills, oracle *Oracle, rl *Limiter, ip string) model {
	st := newStyles(r)

	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = st.accent

	ta := textarea.New()
	ta.Placeholder = "type a message…"
	ta.Prompt = "│ "
	ta.CharLimit = maxInput
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Prompt = st.accent
	ta.FocusedStyle.Text = st.bright
	ta.BlurredStyle.Prompt = st.dim

	vp := viewport.New(0, 0)

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	bar := lipgloss.NormalBorder()
	d.Styles.NormalTitle = st.dim.PaddingLeft(2)
	d.Styles.SelectedTitle = st.accent.Bold(true).
		Border(bar, false, false, false, true).
		BorderForeground(lipgloss.Color("#ffcc33")).
		PaddingLeft(1)
	d.Styles.DimmedTitle = st.muted.PaddingLeft(2)
	d.Styles.FilterMatch = st.bright.Underline(true)

	l := list.New(nil, d, 0, 0)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(true)
	l.Styles.NoItems = st.muted

	return model{
		st:     st,
		store:  store,
		oracle: oracle,
		rl:     rl,
		ip:     ip,
		screen: screenBanner,
		mode:   ModeDoctrine,
		cats:   store.Categories(),
		list:   l,
		ta:     ta,
		vp:     vp,
		sp:     sp,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Tick(1100*time.Millisecond, func(time.Time) tea.Msg { return bannerDoneMsg{} })
}

// ---- commands ----

func loadSystemCmd(store *Skills, mode Mode, skill string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sys, err := store.SystemFor(ctx, mode, skill)
		name := skill
		if mode == ModeDoctrine {
			name = "doctrine"
		}
		return skillLoadedMsg{name: name, system: sys, err: err}
	}
}

func askCmd(o *Oracle, system string, msgs []Msg) tea.Cmd {
	cp := append([]Msg(nil), msgs...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		txt, err := o.Ask(ctx, system, cp)
		return replyMsg{text: txt, err: err}
	}
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case bannerDoneMsg:
		if m.screen == screenBanner {
			m.screen = screenModes
		}
		return m, nil

	case spinner.TickMsg:
		if m.thinking || m.loading {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			if m.thinking {
				m.renderTranscript()
			}
			return m, cmd
		}
		return m, nil

	case skillLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.flash = "load failed: " + short(msg.err)
			m.screen = screenSkills
			return m, nil
		}
		m.skillName = msg.name
		m.system = msg.system
		m.msgs = nil
		m.flash = ""
		m.screen = screenChat
		m.renderTranscript()
		return m, m.ta.Focus()

	case replyMsg:
		m.thinking = false
		switch {
		case msg.err != nil && errors.Is(msg.err, ErrRefused):
			refusal := "[ request refused ]"
			if m.mode != ModeGeneric {
				refusal = "That request lies outside the Covenant."
			}
			m.msgs = append(m.msgs, Msg{Role: "assistant", Content: refusal})
		case msg.err != nil:
			m.msgs = append(m.msgs, Msg{Role: "assistant", Content: "[ error: " + short(msg.err) + " ]"})
		default:
			m.msgs = append(m.msgs, Msg{Role: "assistant", Content: msg.text})
		}
		m.trimHistory()
		m.renderTranscript()
		return m, nil
	}

	switch m.screen {
	case screenBanner:
		if _, ok := msg.(tea.KeyMsg); ok {
			m.screen = screenModes
		}
		return m, nil
	case screenModes:
		return m.updateModes(msg)
	case screenCats:
		return m.updateCats(msg)
	case screenSkills:
		return m.updateSkills(msg)
	case screenChat:
		return m.updateChat(msg)
	}
	return m, nil
}

func (m model) updateModes(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.modeIdx > 0 {
			m.modeIdx--
		}
	case "down", "j":
		if m.modeIdx < len(modeList)-1 {
			m.modeIdx++
		}
	case "enter", "l", "right":
		m.mode = modeList[m.modeIdx].key
		m.flash = ""
		switch m.mode {
		case ModeDoctrine:
			// No skill — load the doctrine prompt and go straight to chat.
			m.loading = true
			m.skillName = "doctrine"
			m.screen = screenChat
			return m, tea.Batch(m.sp.Tick, loadSystemCmd(m.store, m.mode, ""))
		case ModeVoice:
			// Voices are the After— grouping.
			m.loadCategory("After —")
			m.screen = screenSkills
		case ModeGeneric:
			m.screen = screenCats
		}
	}
	return m, nil
}

func (m model) updateCats(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "h", "left":
		m.screen = screenModes
		return m, nil
	case "up", "k":
		if m.catIdx > 0 {
			m.catIdx--
		}
	case "down", "j":
		if m.catIdx < len(m.cats)-1 {
			m.catIdx++
		}
	case "enter", "l", "right":
		if len(m.cats) > 0 {
			m.loadCategory(m.cats[m.catIdx])
			m.screen = screenSkills
		}
	}
	return m, nil
}

func (m model) updateSkills(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		filtering := m.list.FilterState() == list.Filtering
		if !filtering {
			switch km.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "esc", "h", "left":
				// Voice skipped the category menu; go back to mode select.
				if m.mode == ModeVoice {
					m.screen = screenModes
				} else {
					m.screen = screenCats
				}
				m.flash = ""
				return m, nil
			case "enter":
				if it, ok := m.list.SelectedItem().(skillItem); ok {
					m.loading = true
					m.skillName = it.name
					m.screen = screenChat
					m.flash = ""
					return m, tea.Batch(m.sp.Tick, loadSystemCmd(m.store, m.mode, it.name))
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// back out — reset the conversation. Doctrine has no skill list, so
			// it returns to the mode picker; voice/generic return to the skills.
			if m.mode == ModeDoctrine {
				m.screen = screenModes
			} else {
				m.screen = screenSkills
			}
			m.msgs = nil
			m.system = ""
			m.ta.Reset()
			m.ta.Blur()
			return m, nil
		case "enter":
			if m.loading || m.thinking {
				return m, nil
			}
			text := strings.TrimSpace(m.ta.Value())
			if text == "" {
				return m, nil
			}
			if ok, reason := m.rl.Allow(m.ip); !ok {
				m.flash = reason
				return m, nil
			}
			if len(text) > maxInput {
				text = text[:maxInput]
			}
			m.flash = ""
			m.msgs = append(m.msgs, Msg{Role: "user", Content: text})
			m.trimHistory()
			m.ta.Reset()
			m.thinking = true
			m.renderTranscript()
			return m, tea.Batch(m.sp.Tick, askCmd(m.oracle, m.system, m.msgs))
		}
	}
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// ---- helpers ----

func (m *model) resize(w, h int) {
	m.width, m.height = w, h

	lw := 40
	if lw > w-28 {
		lw = w - 28
	}
	if lw < 18 {
		lw = 18
	}
	listH := h - 8
	if listH < 3 {
		listH = 3
	}
	m.list.SetSize(lw, listH)
	m.descW = w - lw - 8
	if m.descW < 10 {
		m.descW = 10
	}

	vpW := w - 4
	if vpW < 10 {
		vpW = 10
	}
	vpH := h - 9
	if vpH < 3 {
		vpH = 3
	}
	m.vp.Width = vpW
	m.vp.Height = vpH
	m.ta.SetWidth(vpW)
	m.renderTranscript()
}

func (m *model) loadCategory(cat string) {
	skills := m.store.InCategory(cat)
	items := make([]list.Item, len(skills))
	for i, s := range skills {
		items[i] = skillItem{name: s.Name, desc: s.Description}
	}
	m.list.SetItems(items)
	m.list.ResetFilter()
	m.list.Select(0)
	m.activeCat = cat
}

func (m *model) trimHistory() {
	if len(m.msgs) > maxTurns {
		m.msgs = m.msgs[len(m.msgs)-maxTurns:]
	}
}

func (m *model) renderTranscript() {
	w := m.vp.Width
	if w < 10 {
		w = 10
	}
	var b strings.Builder
	for i, msg := range m.msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if msg.Role == "user" {
			b.WriteString(m.st.muted.Render("‹ you"))
			b.WriteString("\n")
			b.WriteString(m.st.dim.Width(w).Render(msg.Content))
		} else {
			b.WriteString(m.st.accent.Render("» " + m.speaker()))
			b.WriteString("\n")
			b.WriteString(m.st.bright.Width(w).Render(msg.Content))
		}
	}
	if m.thinking {
		if len(m.msgs) > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.st.accent.Render("» " + m.speaker()))
		b.WriteString("\n")
		b.WriteString(m.st.muted.Render(m.sp.View() + " thinking…"))
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

// speaker is the label shown before an assistant turn.
func (m model) speaker() string {
	switch m.mode {
	case ModeGeneric:
		return m.skillName
	case ModeVoice:
		return "oracle·" + m.skillName
	default:
		return "oracle"
	}
}

// statusLabel is the active-persona line shown above the chat.
func (m model) statusLabel() string {
	switch m.mode {
	case ModeVoice:
		return "oracle · after-voice: " + m.skillName
	case ModeGeneric:
		return "open channel · " + m.skillName
	default:
		return "oracle · doctrine"
	}
}

func short(err error) string {
	s := err.Error()
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

// ---- view ----

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.screen {
	case screenBanner:
		return m.bannerView()
	case screenModes:
		return m.modesView()
	case screenCats:
		return m.catsView()
	case screenSkills:
		return m.skillsView()
	case screenChat:
		return m.chatView()
	}
	return ""
}

func (m model) bannerView() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(m.st.bright.Render("  ┌──────────────────────────────────────────────┐"))
	b.WriteString("\n")
	b.WriteString(m.st.bright.Render("  │") + m.st.inv.Render("    C o C A   ·   S K I L L   T E R M I N A L   ") + m.st.bright.Render("│"))
	b.WriteString("\n")
	b.WriteString(m.st.bright.Render("  └──────────────────────────────────────────────┘"))
	b.WriteString("\n\n")
	b.WriteString(m.st.dim.Render("  > establishing link "))
	b.WriteString(m.st.accent.Render("········· [ OK ]"))
	b.WriteString("\n")
	b.WriteString(m.st.dim.Render(fmt.Sprintf("  > mounting skill modules ··· [ %d ]", m.store.Count())))
	b.WriteString("\n")
	b.WriteString(m.st.dim.Render("  > channel open."))
	b.WriteString(m.st.muted.Render("  press any key…"))
	b.WriteString("\n")
	return b.String()
}

func (m model) bar() string {
	return m.st.dim.Render("══ ") + m.st.bright.Bold(true).Render("CoCA") +
		m.st.dim.Render(" ░ skill terminal ") +
		m.st.muted.Render(fmt.Sprintf("· %d modules", m.store.Count()))
}

func (m model) modesView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	b.WriteString(m.st.muted.Render("  choose a channel") + "\n\n")
	for i, opt := range modeList {
		line := fmt.Sprintf("%-22s %s", opt.label, m.st.muted.Render(opt.blurb))
		if i == m.modeIdx {
			b.WriteString("  " + m.st.accent.Bold(true).Render("▸ "+line) + "\n")
		} else {
			b.WriteString("  " + m.st.dim.Render("  "+line) + "\n")
		}
	}
	b.WriteString("\n")
	if m.flash != "" {
		b.WriteString("  " + m.st.err.Render(m.flash) + "\n\n")
	}
	b.WriteString(m.hint("↑/↓ move   ⏎ select   q disconnect"))
	return b.String()
}

func (m model) catsView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	b.WriteString(m.st.muted.Render("  select a category") + "\n\n")
	for i, c := range m.cats {
		n := len(m.store.InCategory(c))
		line := fmt.Sprintf("%s  %s",
			strings.ToUpper(c),
			m.st.muted.Render(fmt.Sprintf("(%d)", n)))
		if i == m.catIdx {
			b.WriteString("  " + m.st.accent.Bold(true).Render("▸ "+line) + "\n")
		} else {
			b.WriteString("  " + m.st.dim.Render("  "+line) + "\n")
		}
	}
	b.WriteString("\n")
	if m.flash != "" {
		b.WriteString("  " + m.st.err.Render(m.flash) + "\n\n")
	}
	b.WriteString(m.hint("↑/↓ move   ⏎ open   q disconnect"))
	return b.String()
}

func (m model) skillsView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	b.WriteString("  " + m.st.bright.Bold(true).Render(strings.ToUpper(m.activeCat)) + "\n")

	left := m.list.View()

	var desc string
	if it, ok := m.list.SelectedItem().(skillItem); ok {
		desc = m.st.accent.Render(it.name) + "\n\n" + m.st.dim.Width(m.descW).Render(it.desc)
	} else {
		desc = m.st.muted.Render("no match")
	}
	right := lipgloss.NewStyle().Width(m.descW).Render(desc)

	cols := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	b.WriteString(cols)
	b.WriteString("\n")
	b.WriteString(m.hint("↑/↓ move   / filter   ⏎ load module   esc back   q disconnect"))
	return b.String()
}

func (m model) chatView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n")

	if m.loading || m.system == "" {
		b.WriteString("\n  " + m.st.accent.Render(m.sp.View()) +
			m.st.dim.Render(" establishing module "+m.st.bright.Render(m.skillName)+" …") + "\n")
		return b.String()
	}

	status := m.st.muted.Render("[ ") + m.st.accent.Render(m.statusLabel()) + m.st.muted.Render(" ]")
	b.WriteString("  " + status + "\n")
	b.WriteString(m.vp.View() + "\n")
	b.WriteString(m.ta.View() + "\n")
	if m.flash != "" {
		b.WriteString("  " + m.st.err.Render(m.flash) + "\n")
	}
	b.WriteString(m.hint("⏎ send   esc swap module   ctrl+c disconnect"))
	return b.String()
}

func (m model) hint(s string) string {
	return m.st.muted.Render("  " + s)
}
