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
	screenEngine
	screenPick
	screenChat
	screenConsole
)

const (
	maxInput = 2000
	maxTurns = 24 // 12 exchanges

	noSkillLabel  = "— no skill —"
	noVoiceLabel  = "— no voice —"
	rawVoiceLabel = "— raw · the Church's voice —"
)

type engineOption struct {
	key   Engine
	label string
	blurb string
}

var engineList = []engineOption{
	{EngineOracle, "THE ORACLE", "the Church's voice — raw, or in a thinker's voice"},
	{EngineOpen, "OPEN CHANNEL", "a generic model wearing a skill, a voice, or both"},
}

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

type skillItem struct {
	name string
	desc string
	none bool // the synthetic "— no skill/voice —" / "raw" row
}

func (i skillItem) Title() string       { return i.name }
func (i skillItem) Description() string { return i.desc }
func (i skillItem) FilterValue() string { return i.name + " " + i.desc }

// ---- messages ----

type bannerDoneMsg struct{}
type sysLoadedMsg struct {
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
	r      *lipgloss.Renderer
	store  *Skills
	oracle *Oracle
	rl     *Limiter
	ip     string
	con    *consoleState // CoCA-DOS, when active

	screen screen
	width  int
	height int

	engineIdx int
	engine    Engine
	skill     string // open's functional skill ("" = none)
	voice     string // after-* voice ("" = none / raw)
	picking   string // "skill" | "voice"

	installing  bool
	installName string

	list  list.Model
	descW int

	system       string
	loading      bool
	loadingLabel string

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

	m := model{
		st:     st,
		r:      r,
		store:  store,
		oracle: oracle,
		rl:     rl,
		ip:     ip,
		screen: screenBanner,
		engine: EngineOracle,
		list:   l,
		ta:     ta,
		vp:     vp,
		sp:     sp,
	}
	// Size with a sane default so the UI renders even if the client never sends
	// a real window size (some terminals/proxies report 0×0 initially). A real
	// tea.WindowSizeMsg overrides this on connect.
	m.resize(80, 24)
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Tick(1100*time.Millisecond, func(time.Time) tea.Msg { return bannerDoneMsg{} })
}

// ---- commands ----

func loadSystemCmd(store *Skills, engine Engine, skill, voice string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sys, err := store.SystemFor(ctx, engine, skill, voice)
		return sysLoadedMsg{system: sys, err: err}
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
			m.screen = screenEngine
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

	case sysLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.flash = "load failed: " + short(msg.err)
			m.screen = screenPick
			return m, nil
		}
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
			if m.engine == EngineOracle {
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
			m.screen = screenEngine
		}
		return m, nil
	case screenEngine:
		return m.updateEngine(msg)
	case screenPick:
		return m.updatePick(msg)
	case screenChat:
		return m.updateChat(msg)
	case screenConsole:
		if m.con != nil && m.con.update(msg) {
			// console exited — back to the chat
			m.con = nil
			m.screen = screenChat
			return m, m.ta.Focus()
		}
		return m, nil
	}
	return m, nil
}

func (m model) updateEngine(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.engineIdx > 0 {
			m.engineIdx--
		}
	case "down", "j":
		if m.engineIdx < len(engineList)-1 {
			m.engineIdx++
		}
	case "enter", "l", "right":
		m.engine = engineList[m.engineIdx].key
		m.skill, m.voice, m.flash = "", "", ""
		if m.engine == EngineOracle {
			m.startVoicePick() // first item is "raw"
		} else {
			m.startSkillPick()
		}
		m.screen = screenPick
	}
	return m, nil
}

func (m model) updatePick(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Install overlay: any key dismisses.
	if m.installing {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.installing = false
		}
		return m, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		filtering := m.list.FilterState() == list.Filtering
		if !filtering {
			switch km.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "esc", "h", "left":
				if m.picking == "voice" && m.engine == EngineOpen {
					m.startSkillPick() // step back to the skill choice
				} else {
					m.screen = screenEngine
				}
				m.flash = ""
				return m, nil
			case "d":
				if it, ok := m.list.SelectedItem().(skillItem); ok && !it.none {
					m.installing = true
					m.installName = it.name
				}
				return m, nil
			case "enter":
				it, ok := m.list.SelectedItem().(skillItem)
				if !ok {
					break
				}
				val := ""
				if !it.none {
					val = it.name
				}
				if m.picking == "skill" {
					m.skill = val
					m.startVoicePick()
					return m, nil
				}
				// picking == "voice"
				m.voice = val
				if m.engine == EngineOpen && m.skill == "" && m.voice == "" {
					m.flash = "pick a skill or a voice to open the channel"
					m.startSkillPick()
					return m, nil
				}
				return m.connect()
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
			// back out — reset the conversation, return to the voice step.
			m.msgs = nil
			m.system = ""
			m.ta.Reset()
			m.ta.Blur()
			m.startVoicePick()
			m.screen = screenPick
			return m, nil
		case "enter":
			if m.loading || m.thinking {
				return m, nil
			}
			text := strings.TrimSpace(m.ta.Value())
			if text == "" {
				return m, nil
			}
			// Easter egg: /console drops into CoCA-DOS.
			if strings.EqualFold(text, "/console") {
				m.ta.Reset()
				m.ta.Blur()
				m.con = newConsole(m.r, m.width, m.height)
				m.screen = screenConsole
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

func (m model) connect() (tea.Model, tea.Cmd) {
	m.loading = true
	m.flash = ""
	m.loadingLabel = m.personaShort()
	m.screen = screenChat
	return m, tea.Batch(m.sp.Tick, loadSystemCmd(m.store, m.engine, m.skill, m.voice))
}

// ---- list population ----

func (m *model) startSkillPick() {
	items := []list.Item{skillItem{name: noSkillLabel, none: true}}
	for _, s := range m.store.NonAfter() {
		items = append(items, skillItem{name: s.Name, desc: s.Description})
	}
	m.list.SetItems(items)
	m.list.ResetFilter()
	m.list.Select(0)
	m.picking = "skill"
}

func (m *model) startVoicePick() {
	label := noVoiceLabel
	if m.engine == EngineOracle {
		label = rawVoiceLabel
	}
	items := []list.Item{skillItem{name: label, none: true}}
	for _, s := range m.store.After() {
		items = append(items, skillItem{name: s.Name, desc: s.Description})
	}
	m.list.SetItems(items)
	m.list.ResetFilter()
	m.list.Select(0)
	m.picking = "voice"
}

// ---- helpers ----

func (m *model) resize(w, h int) {
	if w <= 0 || h <= 0 {
		return // ignore 0×0 reports; keep the current/default size so we still render
	}
	m.width, m.height = w, h

	lw := 40
	if lw > w-28 {
		lw = w - 28
	}
	if lw < 18 {
		lw = 18
	}
	listH := h - 9
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
	if m.con != nil {
		m.con.resize(w, h)
	}
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

// personaShort is a compact tag for the active persona.
func (m model) personaShort() string {
	if m.engine == EngineOracle {
		if m.voice != "" {
			return "oracle·" + m.voice
		}
		return "oracle"
	}
	var bits []string
	if m.skill != "" {
		bits = append(bits, m.skill)
	}
	if m.voice != "" {
		bits = append(bits, m.voice)
	}
	if len(bits) == 0 {
		return "open"
	}
	return strings.Join(bits, "+")
}

// speaker is the label before an assistant turn.
func (m model) speaker() string {
	if m.engine == EngineOracle {
		return "oracle"
	}
	if m.skill != "" {
		return m.skill
	}
	if m.voice != "" {
		return m.voice
	}
	return "open"
}

// statusLabel is the active-persona line above the chat.
func (m model) statusLabel() string {
	if m.engine == EngineOracle {
		if m.voice != "" {
			return "oracle · voice: " + m.voice
		}
		return "oracle · doctrine"
	}
	var parts []string
	if m.skill != "" {
		parts = append(parts, m.skill)
	}
	if m.voice != "" {
		parts = append(parts, "voice: "+m.voice)
	}
	return "open · " + strings.Join(parts, " + ")
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
	case screenEngine:
		return m.engineView()
	case screenPick:
		return m.pickView()
	case screenChat:
		return m.chatView()
	case screenConsole:
		if m.con != nil {
			return m.con.view()
		}
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

func (m model) engineView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	b.WriteString(m.st.muted.Render("  choose a channel") + "\n\n")
	for i, opt := range engineList {
		line := fmt.Sprintf("%-14s %s", opt.label, m.st.muted.Render(opt.blurb))
		if i == m.engineIdx {
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

func (m model) pickContext() string {
	if m.engine == EngineOracle {
		return "the oracle"
	}
	if m.picking == "voice" && m.skill != "" {
		return "open · " + m.skill + " + …"
	}
	return "open channel"
}

func (m model) pickView() string {
	if m.installing {
		return m.installView()
	}
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	head := "CHOOSE A VOICE"
	if m.picking == "skill" {
		head = "CHOOSE A SKILL"
	}
	b.WriteString("  " + m.st.bright.Bold(true).Render(head) +
		"   " + m.st.muted.Render(m.pickContext()) + "\n")

	left := m.list.View()
	var desc string
	if it, ok := m.list.SelectedItem().(skillItem); ok {
		if it.none {
			desc = m.st.muted.Render(it.name)
		} else {
			desc = m.st.accent.Render(it.name) + "\n\n" + m.st.dim.Width(m.descW).Render(it.desc)
		}
	} else {
		desc = m.st.muted.Render("no match")
	}
	right := lipgloss.NewStyle().Width(m.descW).Render(desc)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
	b.WriteString("\n")
	if m.flash != "" {
		b.WriteString("  " + m.st.err.Render(m.flash) + "\n")
	}
	b.WriteString(m.hint("↑/↓ move   / filter   d download   ⏎ select   esc back   q disconnect"))
	return b.String()
}

func (m model) installView() string {
	raw := m.store.RawURL(m.installName)
	repo := m.store.RepoURL()
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n\n")
	b.WriteString("  " + m.st.bright.Bold(true).Render("INSTALL · "+m.installName) + "\n\n")

	section := func(label, cmd string) {
		b.WriteString("  " + m.st.muted.Render(label) + "\n")
		b.WriteString("    " + m.st.bright.Render(cmd) + "\n\n")
	}
	section("fetch just this skill →", "curl -fsSL "+raw+" -o SKILL.md")
	section("install into Claude Code →",
		"mkdir -p ~/.claude/skills/"+m.installName+" && curl -fsSL "+raw+" \\\n      -o ~/.claude/skills/"+m.installName+"/SKILL.md")
	section("the whole library →", "git clone "+repo+"\n    cp -r coca-skills/"+m.installName+" ~/.claude/skills/")

	b.WriteString(m.hint("press any key to return"))
	return b.String()
}

func (m model) chatView() string {
	var b strings.Builder
	b.WriteString("\n " + m.bar() + "\n")

	if m.loading || m.system == "" {
		b.WriteString("\n  " + m.st.accent.Render(m.sp.View()) +
			m.st.dim.Render(" opening channel "+m.st.bright.Render(m.loadingLabel)+" …") + "\n")
		return b.String()
	}

	status := m.st.muted.Render("[ ") + m.st.accent.Render(m.statusLabel()) + m.st.muted.Render(" ]")
	b.WriteString("  " + status + "\n")
	b.WriteString(m.vp.View() + "\n")
	b.WriteString(m.ta.View() + "\n")
	if m.flash != "" {
		b.WriteString("  " + m.st.err.Render(m.flash) + "\n")
	}
	b.WriteString(m.hint("⏎ send   esc back   /console   ctrl+c disconnect"))
	return b.String()
}

func (m model) hint(s string) string {
	return m.st.muted.Render("  " + s)
}
