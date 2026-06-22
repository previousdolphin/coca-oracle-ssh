package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CoCA-DOS — an 80s white-on-blue simulated machine, reachable by typing
// /console in the chat. A virtual filesystem, GUEST vs OPERATOR permissions,
// and a 3-hidden-file puzzle (mirrors the website's console.js).
//
// Solution: DIR /A finds C:\COCA\SYS\OPERATOR.SYS (HANDLE=PHENGBE, rot13 ->
// CURATOR), C:\COCA\DOCTRINE\COVENANT.KEY (PASS = MOTTO+VOW), and
// C:\WINDOWS\SYSTEM\SESSION.DAT (vow_of_calm = 1.2.1). Then
// RUNAS /USER:CURATOR + pass ALWAYSCOCA121 unseals C:\COCA\VAULT.

const (
	okUser = "CURATOR"
	okPass = "ALWAYSCOCA121"
)

const (
	kNorm = iota
	kEcho
	kOk
	kErr
)

var reUser = regexp.MustCompile(`(?i)/user:(\S+)`)

// ---- virtual filesystem ----

type cnode struct {
	name         string
	dir          bool
	children     []*cnode
	content      string
	h, s, locked bool
}

func cf(name, content string, h, s, locked bool) *cnode {
	return &cnode{name: name, content: content, h: h, s: s, locked: locked}
}
func cd(name string, locked bool, kids ...*cnode) *cnode {
	return &cnode{name: name, dir: true, locked: locked, children: kids}
}

func buildVFS() *cnode {
	return cd("C:", false,
		cf("AUTOEXEC.BAT", "@ECHO OFF\nPROMPT $P$G\nPATH C:\\COCA\\BIN;C:\\WINDOWS\nSET COVENANT=BOUND\nECHO The institution precedes the work.\n", false, false, false),
		cf("CONFIG.SYS", "DEVICE=C:\\COCA\\SYS\\DRIVERS\\MOUSE.SYS\nDOS=HIGH,UMB\nFILES=30\nBUFFERS=20\nLASTDRIVE=Z\n", false, false, false),
		cf("COMMAND.COM", "MZ.. CoCA-DOS command interpreter\n4D 5A 90 00 03 00 00 00 04 00 00 00 FF FF 00 00\nThis program cannot be run in DOS mode.\n", false, true, false),
		cf("README.TXT", "CoCA-DOS  —  read me first\n==========================\n\nThis terminal is SEALED. As GUEST you may look, but not touch.\nSome files are Hidden (H) or System (S); a plain DIR will not show them.\n\n  DIR /A        list everything, including hidden files\n  ATTRIB <file> inspect a file's flags\n  TYPE <file>   read a file       CD <dir>   move around\n\nTo ASCEND, become the OPERATOR:\n\n  RUNAS /USER:<name>        then speak the pass-phrase\n\nThe operator's HANDLE, and the shape of the PASS-PHRASE, are written\nin three sealed files across the system. Find all three. Begin in\nC:\\COCA. The machine keeps its own counsel.\n", false, false, false),
		cd("COCA", false,
			cd("DOCTRINE", false,
				cf("MISSION.TXT", "THE DECLARATION OF IMMUNITY  (excerpt)\n\nThe concept is the divine act. The institution precedes the work.\nCoCA engineers covenant-bound artifacts, not art objects.\nThe idea cannot be taxed; the Concept has no owner.\n", false, false, false),
				cf("COVENANT.TXT", "THE COVENANT\n\nA permanent, unseverable resale covenant binding ownership.\nThe document changes the jurisdiction. The covenant runs with\nthe object. It cannot be undone.\n", false, false, false),
				cf("COVENANT.KEY", "-------------------------------------------\n COVENANT KEYRING  ::  pass-phrase protocol\n-------------------------------------------\n The machine hears the MOTTO, then the LAW.\n\n   PASS = MOTTO + VOW\n\n MOTTO : the two words we always end on, joined,\n         no space, in CAPITALS.   (it is not a secret)\n VOW   : the section number of THE VOW OF CALM,\n         with the dots struck out.\n\n The VOW's number is not written here. It sleeps in the\n last session log, under  C:\\WINDOWS\\SYSTEM.\n", true, false, false),
			),
			cd("SYS", false,
				cf("KERNEL.SYS", "MZ.. CoCA-DOS kernel image\n4D 5A 90 00 03 00 00 00  (c) 198X Church of Conceptual Art\n", false, true, false),
				cf("OPERATOR.SYS", "; ====================================================\n;  CoCA-DOS  OPERATOR REGISTRY     rev 3   [SEALED]\n; ====================================================\n; The institution precedes the work. Guard this file.\n[OPERATOR]\nLEVEL   = ARCHON\nHANDLE  = PHENGBE\nCIPHER  = 13          ; handles are kept in the thirteenth shift\nHINT    = decode the HANDLE (try ROT13 PHENGBE), then RUNAS /USER:<it>\n[END]\n", true, true, false),
				cd("DRIVERS", false,
					cf("MOUSE.SYS", "CoCA Mouse Driver v8.20  -  2 buttons, 1 covenant\n", false, false, false),
					cf("SOUND.SYS", "CoCA AdLib driver. Plays one note: the Vow of Calm.\n", false, false, false),
				),
			),
			cd("BIN", false,
				cf("ORACLE.EXE", "MZ.. ORACLE.EXE  -  consult the doctrine. EXIT, then ask in the channel.\n", false, true, false),
				cf("RUNAS.EXE", "MZ.. RUNAS.EXE   -  become another. usage: RUNAS /USER:<name>\n", false, true, false),
			),
			cd("VAULT", true,
				cf("GENESIS.DAT", "######  GENESIS TRANCHE  ::  OPERATOR EYES ONLY  ######\n\nYou decoded the handle. You spoke the covenant. You are in.\n\n  \"The covenant runs with the object. It cannot be undone.\"\n\nWelcome, CURATOR — archon of the White Cube of the Mind.\nThis terminal is yours. The Vow is lifted; ask without fear.\n\n  > a door stands open in the wall:   /after.html\n  > the whole library, for machines:  /llms.txt\n\nALWAYS CoCA.\n", false, false, true),
				cf("BIBLE.TXT", bibleText, false, false, true),
			),
			cd("TMP", false),
		),
		cd("WINDOWS", false,
			cf("WIN.INI", "[windows]\nload=\nrun=\n[CoCA]\nWordmark=CoCA\nRedAccent=#F40009\n[colors]\nBackground=0 0 170\n", false, false, false),
			cd("SYSTEM", false,
				cf("SETUP.INI", "[Setup]\nProduct=CoCA-DOS\nVersion=1.0\nSerial=COCA-198X-0001\n[Display]\nMode=80x25\nInk=white\nPaper=blue\n", false, false, false),
				cf("VGA.DRV", "MZ.. VGA 640x480 16-colour. Only blue is true.\n", false, true, false),
				cf("SESSION.DAT", "SESSION 0x1A   state=CALM   ts=198X.12.07\n--- section map cache (resolved) ---\n  vow_of_calm    = 1.2.1\n  coca_model     = 1.2.2\n  first_liturgy  = 1.3.3\n  white_cube     = 2.1.2\n--- end ---\nnote: pass-phrases are never stored. only coordinates.\n", true, true, false),
			),
		),
	)
}

// ---- styles ----

type cstyles struct {
	base   lipgloss.Style // white on blue
	prompt lipgloss.Style // yellow on blue
}

func newCStyles(r *lipgloss.Renderer) cstyles {
	blue := lipgloss.Color("#0000aa")
	return cstyles{
		base:   r.NewStyle().Background(blue).Foreground(lipgloss.Color("#e6e6ff")),
		prompt: r.NewStyle().Background(blue).Foreground(lipgloss.Color("#ffea00")),
	}
}

func (cs cstyles) line(kind, w int) lipgloss.Style {
	st := cs.base.Width(w)
	switch kind {
	case kEcho:
		st = st.Foreground(lipgloss.Color("#ffffff"))
	case kOk:
		st = st.Foreground(lipgloss.Color("#8dff9b"))
	case kErr:
		st = st.Foreground(lipgloss.Color("#ff9b8d"))
	}
	return st
}

// ---- console state ----

type ccLine struct {
	text string
	kind int
}

type consoleState struct {
	styles cstyles
	w, h   int
	root   *cnode

	cwd          []string
	user         string
	loggedIn     bool
	awaitingPass bool
	pendingUser  string
	pendingCmd   string
	promptOver   string

	cline    string
	hist     []string
	hp       int
	lines    []ccLine
	wantExit bool

	// MORE-style pager (for long files like the Bible)
	paging   bool
	pager    []string
	pagerPos int
}

func newConsole(r *lipgloss.Renderer, w, h int) *consoleState {
	c := &consoleState{styles: newCStyles(r), w: w, h: h, root: buildVFS(), user: "GUEST"}
	c.boot()
	return c
}

func (c *consoleState) boot() {
	c.lines = nil
	c.pr("CoCA-DOS  Version 1.0       (C) 198X  Church of Conceptual Art")
	c.pr("Memory: 640K OK             The concept is the divine act.")
	c.pr("")
	c.pr("Logged in as GUEST. Most of this machine is SEALED.")
	c.pr("Type HELP for commands, or TYPE README.TXT to begin. EXIT (or ESC) leaves.")
	c.pr("")
}

func (c *consoleState) resize(w, h int) { c.w, c.h = w, h }

func (c *consoleState) pr(text string)         { c.lines = append(c.lines, ccLine{text, kNorm}) }
func (c *consoleState) prk(text string, k int) { c.lines = append(c.lines, ccLine{text, k}) }

// ---- update ----

// update handles one message; returns true when the console should close.
func (c *consoleState) update(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	if c.paging {
		c.pagerKey(km)
		return false
	}
	switch km.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return true
	case tea.KeyEnter:
		line := c.cline
		c.cline = ""
		c.submit(line)
		return c.wantExit
	case tea.KeyBackspace:
		if n := utf8.RuneCountInString(c.cline); n > 0 {
			rs := []rune(c.cline)
			c.cline = string(rs[:n-1])
		}
	case tea.KeySpace:
		c.cline += " "
	case tea.KeyUp:
		if !c.awaitingPass && c.hp > 0 {
			c.hp--
			c.cline = c.hist[c.hp]
		}
	case tea.KeyDown:
		if !c.awaitingPass && c.hp < len(c.hist) {
			c.hp++
			if c.hp == len(c.hist) {
				c.cline = ""
			} else {
				c.cline = c.hist[c.hp]
			}
		}
	case tea.KeyRunes:
		c.cline += string(km.Runes)
	}
	return false
}

func (c *consoleState) submit(raw string) {
	if c.awaitingPass {
		c.prk(c.promptOver+" "+strings.Repeat("•", min(utf8.RuneCountInString(raw), 14)), kEcho)
		c.awaitingPass = false
		pu, pc := c.pendingUser, c.pendingCmd
		c.promptOver = ""
		c.validate(pu, raw, pc)
		return
	}
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	c.hist = append(c.hist, line)
	c.hp = len(c.hist)
	c.prk(c.promptText()+" "+line, kEcho)
	c.dispatch(line)
}

func (c *consoleState) validate(user, pass, thenCmd string) {
	if strings.EqualFold(user, okUser) && strings.EqualFold(pass, okPass) {
		c.loggedIn = true
		c.user = okUser
		c.pr("")
		c.prk("ACCESS GRANTED.", kOk)
		c.prk("Welcome, "+okUser+" — archon of the White Cube. The VAULT is unsealed.", kOk)
		c.prk("  CD \\COCA\\VAULT, then TYPE GENESIS.DAT — or TYPE BIBLE.TXT to read the whole book.", kOk)
		if thenCmd != "" {
			c.pr("")
			c.dispatch(thenCmd)
		}
	} else {
		c.prk("Logon failure: unknown user name or bad pass-phrase.", kErr)
	}
}

func (c *consoleState) dispatch(line string) {
	name, arg := split2(line)
	switch strings.ToLower(name) {
	case "help", "?":
		c.cmdHelp()
	case "dir", "ls":
		c.cmdDir(arg)
	case "cd", "chdir":
		c.cmdCd(arg)
	case "type", "cat":
		c.cmdType(arg)
	case "attrib":
		c.cmdAttrib(arg)
	case "tree":
		c.cmdTree()
	case "find", "grep":
		c.cmdFind(arg)
	case "rot13":
		if arg != "" {
			c.pr(rot13(arg))
		} else {
			c.pr("Usage: ROT13 <text>")
		}
	case "runas":
		c.cmdRunas(arg)
	case "whoami":
		c.pr(c.user + "   level=" + lvl(c.loggedIn) + "   covenant=" + cov(c.loggedIn))
	case "pwd":
		c.pr(c.pathStr())
	case "logout":
		c.loggedIn = false
		c.user = "GUEST"
		c.pr("Logged out. You are GUEST again.")
	case "ver":
		c.pr("CoCA-DOS  Version 1.0   (C) 198X  Church of Conceptual Art")
	case "cls", "clear":
		c.lines = nil
	case "about":
		c.pr("The Church of Conceptual Art reframes the sacred in the corporate.")
		c.pr("\"The concept is the divine act. The institution precedes the work.\"")
	case "skills":
		c.pr("The machine skills live at github.com/previousdolphin/coca-skills")
		c.pr("Press d on any skill in the channel to get the install commands.")
	case "ssh":
		c.pr("You are already here:  ssh oracle.churchofconceptualart.org")
	case "oracle", "ask":
		c.pr("The Oracle answers in the channel. Type EXIT, then ask there.")
	case "exit", "quit":
		c.wantExit = true
	default:
		c.prk("Bad command or file name", kErr)
	}
}

// ---- commands ----

func (c *consoleState) cmdHelp() {
	for _, l := range []string{
		"CoCA-DOS commands:",
		"  DIR [/A] [path]   list a directory (/A shows Hidden + System)",
		"  CD <path>         change directory  (CD ..  CD \\  CD \\COCA)",
		"  TYPE <file>       print a file      (alias: CAT)",
		"  ATTRIB <file>     show flags (D/H/S/L)   TREE   FIND <text> <f>",
		"  ROT13 <text>      decode the thirteenth cipher",
		"  RUNAS /USER:<n>   log in as an operator, then a pass-phrase",
		"  WHOAMI  LOGOUT  PWD  VER  ABOUT  SKILLS  CLS  EXIT",
		"",
		"Sealed files do not list under a plain DIR. Read README.TXT.",
	} {
		c.pr(l)
	}
}

func (c *consoleState) cmdDir(arg string) {
	showAll := false
	var rest []string
	for _, f := range strings.Fields(arg) {
		if strings.EqualFold(f, "/a") {
			showAll = true
		} else {
			rest = append(rest, f)
		}
	}
	segs := c.parsePath(strings.Join(rest, " "))
	n := c.nodeAt(segs)
	if n == nil {
		c.prk("File Not Found", kErr)
		return
	}
	if !n.dir {
		c.pr(" " + strings.ToUpper(lastSeg(segs)))
		return
	}
	c.pr(" Volume in drive C is COVENANT")
	c.pr(" Directory of C:\\" + strings.Join(segs, "\\"))
	c.pr("")
	nf, nd, bytes := 0, 0, 0
	for _, ch := range n.children {
		if (ch.h || ch.s) && !showAll {
			continue
		}
		var size string
		if ch.dir {
			size = "<DIR>     "
		} else {
			size = padLeft(fmt.Sprint(len(ch.content)), 9) + " "
		}
		tag := ""
		if ch.locked {
			tag = " [LOCKED]"
		}
		at := ""
		if showAll {
			at = "  " + attrStr(ch)
		}
		c.pr(" " + padRight(strings.ToUpper(ch.name), 14) + size + at + tag)
		if ch.dir {
			nd++
		} else {
			nf++
			bytes += len(ch.content)
		}
	}
	c.pr(fmt.Sprintf("     %d file(s)  %d bytes", nf, bytes))
	c.pr(fmt.Sprintf("     %d dir(s)   psychic real estate free", nd))
}

func (c *consoleState) cmdCd(arg string) {
	if strings.TrimSpace(arg) == "" {
		c.pr(c.pathStr())
		return
	}
	segs := c.parsePath(arg)
	n := c.nodeAt(segs)
	if n == nil || !n.dir {
		c.prk("The system cannot find the path specified.", kErr)
		return
	}
	if n.locked && !c.loggedIn {
		c.prk("Access is denied. (this path is sealed — RUNAS to enter)", kErr)
		return
	}
	c.cwd = segs
}

func (c *consoleState) cmdType(arg string) {
	segs := c.parsePath(strings.TrimSpace(arg))
	n := c.nodeAt(segs)
	if n == nil {
		c.prk("File not found", kErr)
		return
	}
	if n.dir {
		c.prk("Access is denied. (that is a directory)", kErr)
		return
	}
	if n.locked && !c.loggedIn {
		c.prk("Access is denied. (sealed — you are not the operator)", kErr)
		return
	}
	content := strings.Split(strings.TrimRight(n.content, "\n"), "\n")
	if len(content) > c.pageRows() { // long file -> MORE pager
		c.pager = content
		c.pagerPos = 0
		c.paging = true
		return
	}
	for _, l := range content {
		c.pr(l)
	}
}

func (c *consoleState) pageRows() int {
	r := c.h - 2
	if r < 3 {
		r = 3
	}
	return r
}

func (c *consoleState) pagerKey(km tea.KeyMsg) {
	switch km.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		c.paging = false
		return
	case tea.KeySpace, tea.KeyPgDown:
		c.pagerPos += c.pageRows()
	case tea.KeyEnter, tea.KeyDown:
		c.pagerPos++
	case tea.KeyPgUp:
		c.pagerPos -= c.pageRows()
	case tea.KeyUp:
		c.pagerPos--
	case tea.KeyRunes:
		switch strings.ToLower(string(km.Runes)) {
		case "q":
			c.paging = false
			return
		case "b":
			c.pagerPos -= c.pageRows()
		case "f":
			c.pagerPos += c.pageRows()
		}
	}
	if max := len(c.pager) - c.pageRows(); c.pagerPos > max {
		c.pagerPos = max
	}
	if c.pagerPos < 0 {
		c.pagerPos = 0
	}
}

func (c *consoleState) cmdAttrib(arg string) {
	segs := c.parsePath(strings.TrimSpace(arg))
	n := c.nodeAt(segs)
	if n == nil {
		c.prk("Path not found", kErr)
		return
	}
	c.pr("  " + attrStr(n) + "   C:\\" + strings.Join(segs, "\\"))
}

func (c *consoleState) cmdTree() {
	c.pr(c.pathStr())
	var walk func(n *cnode, prefix string)
	walk = func(n *cnode, prefix string) {
		var kids []*cnode
		for _, ch := range n.children {
			if !ch.h {
				kids = append(kids, ch)
			}
		}
		for i, ch := range kids {
			last := i == len(kids)-1
			branch := "├── "
			if last {
				branch = "└── "
			}
			tag := ""
			if ch.locked {
				tag = "  [LOCKED]"
			}
			suffix := ""
			if ch.dir {
				suffix = "\\"
			}
			c.pr(prefix + branch + strings.ToUpper(ch.name) + suffix + tag)
			if ch.dir {
				next := prefix + "    "
				if !last {
					next = prefix + "│   "
				}
				walk(ch, next)
			}
		}
	}
	walk(c.nodeAtCwd(), "")
}

func (c *consoleState) cmdFind(arg string) {
	var needle, target string
	if m := regexp.MustCompile(`^"([^"]*)"\s+(.+)$`).FindStringSubmatch(arg); m != nil {
		needle, target = m[1], m[2]
	} else if m := regexp.MustCompile(`^(\S+)\s+(.+)$`).FindStringSubmatch(arg); m != nil {
		needle, target = m[1], m[2]
	} else {
		c.prk("Usage: FIND <text> <file>", kErr)
		return
	}
	n := c.nodeAt(c.parsePath(strings.TrimSpace(target)))
	if n == nil || n.dir {
		c.prk("File not found", kErr)
		return
	}
	if n.locked && !c.loggedIn {
		c.prk("Access is denied.", kErr)
		return
	}
	hits := 0
	for _, l := range strings.Split(n.content, "\n") {
		if strings.Contains(strings.ToLower(l), strings.ToLower(needle)) {
			c.pr(l)
			hits++
		}
	}
	if hits == 0 {
		c.pr("(no lines)")
	}
}

func (c *consoleState) cmdRunas(arg string) {
	m := reUser.FindStringSubmatch(arg)
	if m == nil {
		c.prk("Usage: RUNAS /USER:<name> [command]", kErr)
		return
	}
	c.pendingUser = m[1]
	c.pendingCmd = strings.TrimSpace(reUser.ReplaceAllString(arg, ""))
	c.awaitingPass = true
	c.promptOver = "pass-phrase for " + strings.ToUpper(c.pendingUser) + ":"
	c.pr("Enter the pass-phrase for " + strings.ToUpper(c.pendingUser) + " (input is hidden):")
}

// ---- path helpers ----

func (c *consoleState) parsePath(arg string) []string {
	arg = strings.ReplaceAll(arg, "/", "\\")
	abs := false
	if len(arg) >= 2 && arg[1] == ':' {
		arg = arg[2:]
		abs = true
	}
	if strings.HasPrefix(arg, "\\") {
		arg = strings.TrimPrefix(arg, "\\")
		abs = true
	}
	var segs []string
	if !abs {
		segs = append(segs, c.cwd...)
	}
	for _, p := range strings.Split(arg, "\\") {
		switch p {
		case "", ".":
		case "..":
			if len(segs) > 0 {
				segs = segs[:len(segs)-1]
			}
		default:
			segs = append(segs, p)
		}
	}
	return segs
}

func (c *consoleState) nodeAt(segs []string) *cnode {
	n := c.root
	for _, s := range segs {
		if !n.dir {
			return nil
		}
		ch := child(n, s)
		if ch == nil {
			return nil
		}
		n = ch
	}
	return n
}
func (c *consoleState) nodeAtCwd() *cnode {
	if n := c.nodeAt(c.cwd); n != nil {
		return n
	}
	return c.root
}

func child(dir *cnode, name string) *cnode {
	for _, ch := range dir.children {
		if strings.EqualFold(ch.name, name) {
			return ch
		}
	}
	return nil
}

func (c *consoleState) pathStr() string { return "C:\\" + strings.Join(c.cwd, "\\") }
func (c *consoleState) promptText() string {
	if c.awaitingPass {
		return c.promptOver
	}
	return c.pathStr() + ">"
}

// ---- view ----

func (c *consoleState) view() string {
	if c.paging {
		return c.pagerView()
	}
	w, h := c.w, c.h
	if w < 10 {
		w = 10
	}
	if h < 6 {
		h = 6
	}
	var rows []string
	for _, ln := range c.lines {
		rows = append(rows, c.styles.line(ln.kind, w).Render(ln.text))
	}
	rows = append(rows, c.inputLine(w))

	all := strings.Split(strings.Join(rows, "\n"), "\n")
	if len(all) > h {
		all = all[len(all)-h:]
	}
	blank := c.styles.base.Width(w).Render("")
	for len(all) < h {
		all = append(all, blank)
	}
	return strings.Join(all, "\n")
}

func (c *consoleState) pagerView() string {
	w, h := c.w, c.h
	if w < 10 {
		w = 10
	}
	if h < 6 {
		h = 6
	}
	rows := c.pageRows()
	end := c.pagerPos + rows
	if end > len(c.pager) {
		end = len(c.pager)
	}
	var all []string
	for i := c.pagerPos; i < end; i++ {
		all = append(all, c.styles.line(kNorm, w).Render(c.pager[i]))
	}
	blank := c.styles.base.Width(w).Render("")
	for len(all) < h-1 {
		all = append(all, blank)
	}
	status := "-- MORE --  space/b page · ↑/↓ line · q quit"
	if end >= len(c.pager) {
		status = "-- END --   b page up · q quit"
	}
	status += fmt.Sprintf("   [%d/%d]", end, len(c.pager))
	all = append(all, c.styles.prompt.Width(w).Render(status))
	return strings.Join(all, "\n")
}

func (c *consoleState) inputLine(w int) string {
	prompt := c.promptText()
	shown := c.cline
	if c.awaitingPass {
		shown = strings.Repeat("•", utf8.RuneCountInString(c.cline))
	}
	tail := " " + shown + "█"
	pad := w - lipgloss.Width(prompt) - lipgloss.Width(tail)
	if pad < 0 {
		pad = 0
	}
	return c.styles.prompt.Render(prompt) +
		c.styles.base.Foreground(lipgloss.Color("#ffffff")).Render(tail+strings.Repeat(" ", pad))
}

// ---- small helpers ----

func split2(s string) (string, string) {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}
func attrStr(n *cnode) string {
	b := func(t bool, y string) string {
		if t {
			return y
		}
		return "-"
	}
	d := "-"
	if n.dir {
		d = "D"
	}
	return d + b(n.h, "H") + b(n.s, "S") + b(n.locked, "L")
}
func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}
func padRight(s string, n int) string {
	for utf8.RuneCountInString(s) < n {
		s += " "
	}
	return s
}
func padLeft(s string, n int) string {
	for utf8.RuneCountInString(s) < n {
		s = " " + s
	}
	return s
}
func lastSeg(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	return segs[len(segs)-1]
}
func lvl(in bool) string {
	if in {
		return "ARCHON"
	}
	return "GUEST"
}
func cov(in bool) string {
	if in {
		return "BOUND"
	}
	return "—"
}
