package editor

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// A paste on Windows comes as plain keys, its line breaks as enter (the key spike, DESIGN
// §23): owner 2026-11-15, pasting `.[].employee` and `| .name` ran the first line alone.
// An enter with more keys straight after it is a line break; one with none runs the line.
func TestPastedEnterIsALineBreak(t *testing.T) {
	m := New("> ")
	m.Burst = 20 * time.Millisecond
	typeInto(m, ".[].employee<enter>| .name")
	if m.Done() {
		t.Fatal("the pasted line break ran the first line")
	}
	if line(m) != ".[].employee\n| .name|" {
		t.Errorf("got %q", line(m))
	}
	// typed: enter, and then nothing until the burst is over
	cmds := typeInto(m, "<enter>")
	if m.Done() {
		t.Fatal("enter ran before the burst was over")
	}
	m.Update(tick(t, cmds[0]))
	if !m.Done() {
		t.Fatal("enter alone didn't run the line")
	}
	if got, _ := m.Result(); got != ".[].employee\n| .name" {
		t.Errorf("ran %q", got)
	}
}

// A tick from an enter that a key has since turned into a line break runs nothing.
func TestStaleBurstTick(t *testing.T) {
	m := New("> ")
	m.Burst = 20 * time.Millisecond
	cmds := typeInto(m, ".a<enter>")
	stale := tick(t, cmds[len(cmds)-1])
	typeInto(m, "| .b")
	m.Update(stale)
	if m.Done() || line(m) != ".a\n| .b|" {
		t.Errorf("a stale tick: done %v, %q", m.Done(), line(m))
	}
}

// A bracketed paste (where the terminal marks one) keeps its line breaks too.
func TestBracketedPasteKeepsLines(t *testing.T) {
	m := New("> ")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".a |\r\n  .b\t"), Paste: true})
	if line(m) != ".a |\n  .b |" {
		t.Errorf("got %q", line(m))
	}
}

// An entry of several lines shows each line after the first under a … prompt, as wide as the
// prompt; the cursor on a line break shows as a reversed space at the end of its line.
func TestViewSeveralLines(t *testing.T) {
	m := New(". ❯ ")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".a\n| .b"), Paste: true})
	if v := m.View(); v != ". ❯ .a\n  … | .b"+reverse+" "+unreverse {
		t.Errorf("got %q", v)
	}
	for range 5 {
		typeInto(m, "<left>")
	}
	if v := m.View(); v != ". ❯ .a"+reverse+" "+unreverse+"\n  … | .b" {
		t.Errorf("cursor on the line break: %q", v)
	}
}

// tick runs a command that should be the burst's timer and gives its message.
func tick(t *testing.T, c tea.Cmd) tea.Msg {
	t.Helper()
	if c == nil {
		t.Fatal("enter gave no timer")
	}
	return c()
}
