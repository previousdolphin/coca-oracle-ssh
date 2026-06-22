package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

func conText(c *consoleState) string {
	var b strings.Builder
	for _, l := range c.lines {
		b.WriteString(l.text)
		b.WriteString("\n")
	}
	return b.String()
}

func TestConsolePuzzle(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	c := newConsole(r, 100, 30)

	if !strings.Contains(c.view(), "CoCA-DOS") {
		t.Fatalf("console view missing boot banner:\n%s", c.view())
	}

	// Plain DIR hides system files; DIR /A reveals them.
	c.lines = nil
	c.submit("dir")
	if strings.Contains(conText(c), "COMMAND.COM") {
		t.Fatal("plain DIR should hide system files")
	}
	c.lines = nil
	c.submit("dir /a")
	if !strings.Contains(conText(c), "COMMAND.COM") {
		t.Fatal("DIR /A should reveal system files")
	}

	// Clue 1: OPERATOR.SYS -> PHENGBE -> rot13 -> CURATOR.
	c.submit("cd \\COCA\\SYS")
	c.lines = nil
	c.submit("dir /a")
	if !strings.Contains(conText(c), "OPERATOR.SYS") {
		t.Fatal("OPERATOR.SYS not listed under DIR /A")
	}
	c.lines = nil
	c.submit("type operator.sys")
	if !strings.Contains(conText(c), "PHENGBE") {
		t.Fatal("operator HANDLE missing")
	}
	if rot13("PHENGBE") != "CURATOR" {
		t.Fatalf("rot13 broken: %q", rot13("PHENGBE"))
	}

	// Clue 3: the vow number.
	c.lines = nil
	c.submit("type \\WINDOWS\\SYSTEM\\SESSION.DAT")
	if !strings.Contains(conText(c), "1.2.1") {
		t.Fatal("vow_of_calm number missing")
	}

	// Vault sealed for GUEST.
	c.lines = nil
	c.submit("cd \\COCA\\VAULT")
	if !strings.Contains(conText(c), "Access is denied") {
		t.Fatal("vault should be denied pre-login")
	}

	// Wrong pass-phrase fails.
	c.lines = nil
	c.submit("runas /user:CURATOR")
	c.submit("NOPE")
	if c.loggedIn || !strings.Contains(conText(c), "Logon failure") {
		t.Fatalf("wrong pass should fail:\n%s", conText(c))
	}

	// Correct (case-insensitive) login unseals the vault.
	c.lines = nil
	c.submit("runas /user:curator")
	c.submit("alwayscoca121")
	if !c.loggedIn || !strings.Contains(conText(c), "ACCESS GRANTED") {
		t.Fatalf("login failed:\n%s", conText(c))
	}
	c.submit("cd \\COCA\\VAULT")
	if c.pathStr() != "C:\\COCA\\VAULT" {
		t.Fatalf("not in vault: %q", c.pathStr())
	}
	c.lines = nil
	c.submit("type GENESIS.DAT")
	if !strings.Contains(conText(c), "You decoded the handle") {
		t.Fatalf("genesis not readable:\n%s", conText(c))
	}

	// BIBLE.TXT is unlocked in the vault (prints, or opens the MORE pager if long).
	c.lines = nil
	c.paging = false
	c.submit("type BIBLE.TXT")
	if !c.paging && !strings.Contains(conText(c), "Concept") {
		t.Fatalf("BIBLE.TXT not readable after login:\n%s", conText(c))
	}

	c.submit("exit")
	if !c.wantExit {
		t.Fatal("EXIT should set wantExit")
	}
}

func TestConsoleTriggerFromChat(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	m := newModel(r, testStore(), NewOracle("", ""), NewLimiter(99, time.Minute, 999), "ip")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(m, bannerDoneMsg{}) // engine
	m = step(m, key("enter"))    // oracle -> voice pick
	m = step(m, key("enter"))    // raw -> connect (loading)
	m = step(m, sysLoadedMsg{system: "SYS"})
	if m.screen != screenChat {
		t.Fatalf("expected chat, got screen %d", m.screen)
	}
	// Typing /console drops into CoCA-DOS rather than asking the Oracle.
	m.ta.SetValue("/console")
	m = step(m, key("enter"))
	if m.screen != screenConsole || m.con == nil {
		t.Fatalf("/console did not open; screen=%d con=%v", m.screen, m.con)
	}
	// ESC exits back to the chat.
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenChat || m.con != nil {
		t.Fatalf("esc should exit console; screen=%d con=%v", m.screen, m.con)
	}
}
