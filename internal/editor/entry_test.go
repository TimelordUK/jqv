package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A unit out for editing (:e name) never walks history: up on the first line stays there
// (owner 2026-11-21: up past the top searched history for the whole unit).
func TestUnitStaysOutOfHistory(t *testing.T) {
	m := New("> ")
	m.Entry = func() EntryKind { return EntryUnit }
	m.History.Add(".old")
	m.Update(pasted(".a\n| .b"))
	typeInto(m, "<up><up><up>")
	if line(m) != ".a|\n| .b" {
		t.Errorf("up past the top: %q", line(m))
	}
	if got := shown(m); !strings.Contains(got, "the first line") {
		t.Errorf("no note:\n%s", got)
	}
	typeInto(m, "<down><down><down>")
	if line(m) != ".a\n| |.b" { // the column kept
		t.Errorf("down past the end: %q", line(m))
	}
}

// The whole program in the editor (:e alone, Q-049): enter breaks a line, esc puts it back,
// and it doesn't go into history.
func TestBufferKeys(t *testing.T) {
	m := New("> ")
	m.Burst = PasteBurst // enter in a buffer breaks the line at once, no burst to wait out
	m.Entry = func() EntryKind { return EntryProgram }
	m.Update(pasted(".a\n| .b"))
	typeInto(m, "<enter>| .c")
	if m.Done() || line(m) != ".a\n| .b\n| .c|" {
		t.Fatalf("enter: done %v, %q", m.Done(), line(m))
	}
	typeInto(m, "<esc>")
	if s, err := m.Result(); !m.Done() || err != nil || s != ".a\n| .b\n| .c" {
		t.Errorf("esc: done %v, %q %v", m.Done(), s, err)
	}
	if len(m.History.Lines()) != 0 {
		t.Errorf("the buffer went into history")
	}
}

// What shows under the entry is one row, however long or many-lined: the window counts on it.
func TestNoteIsOneRow(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m.Update(pasted("x"))
	m.Hinter = func(string, int) string { return strings.Repeat("long ", 30) + "\nand more" }
	got := shown(m)
	if rows := strings.Split(got, "\n"); len(rows) != 2 || len([]rune(rows[1])) > 40 {
		t.Errorf("hint:\n%s", got)
	}
}
