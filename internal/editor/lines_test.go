package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// An entry of several lines (Q-026, DESIGN §23): up and down move between its lines, and
// reach history only from the first or last line; home and end, ctrl+k and ctrl+u, work on
// the line the cursor is on.
func TestMovingInSeveralLines(t *testing.T) {
	m := New("> ")
	m.History.Add(".a\n| .old")
	typeInto(m, ".a`<enter>| .bbbb`<enter>| .c")
	if line(m) != ".a\n| .bbbb\n| .c|" {
		t.Fatalf("got %q", line(m))
	}
	for _, c := range []struct{ keys, want string }{
		{"<up>", ".a\n| .b|bbb\n| .c"},   // the same column
		{"<up>", ".a|\n| .bbbb\n| .c"},   // the end of a shorter line
		{"<down>", ".a\n| |.bbbb\n| .c"}, // the column it is on now
		{"<home>", ".a\n|| .bbbb\n| .c"},
		{"<end>", ".a\n| .bbbb|\n| .c"},
		{"<ctrl+u>", ".a\n|\n| .c"},
		{"<ctrl+k>", ".a\n|| .c"}, // at a line's end: the break, joining the next line on
		{"<ctrl+k>", ".a\n|"},
	} {
		typeInto(m, c.keys)
		if line(m) != c.want {
			t.Errorf("after %s: %q, want %q", c.keys, line(m), c.want)
		}
	}
	typeInto(m, "<up><up>") // up from the first line: history, of entries with the text in them
	if line(m) != ".a\n| .old|" {
		t.Errorf("up on the first line: %q", line(m))
	}
}

// Walking history goes on through an entry of several lines: up again gives the entry before,
// not the line above in this one.
func TestHistoryWalksPastSeveralLines(t *testing.T) {
	m := New("> ")
	m.History.Add(".old")
	m.History.Add(".a\n| .b")
	typeInto(m, "<up>")
	if m.Text() != ".a\n| .b" {
		t.Fatalf("got %q", m.Text())
	}
	typeInto(m, "<up>")
	if m.Text() != ".old" {
		t.Errorf("up again: %q", m.Text())
	}
	// owner 2026-11-15: brought back, the cursor moved, up walks the entry's lines
	typeInto(m, "<down><left><up>")
	if line(m) != ".a|\n| .b" {
		t.Errorf("after moving the cursor, up is the line above: %q", line(m))
	}
}

// A ` and enter on a line before the last opens a new line after it: a stage put in.
func TestBacktickPutsInALine(t *testing.T) {
	m := New("> ")
	m.Update(pasted(".a\n| .c"))
	typeInto(m, "<up><end>`<enter>| .b")
	if m.Done() || line(m) != ".a\n| .b|\n| .c" {
		t.Errorf("done %v, %q", m.Done(), line(m))
	}
}

// Enter at the end of an unfinished entry goes on to a new line and says why; on an empty
// line it runs the entry as it is; a ` at a line's end breaks the line on purpose.
func TestEnterContinues(t *testing.T) {
	m := New("> ")
	m.Continues = func(s string) string {
		if strings.Count(s, "(") > strings.Count(s, ")") {
			return "open: ("
		}
		return ""
	}
	typeInto(m, "select(.a<enter>")
	if m.Done() || line(m) != "select(.a\n|" {
		t.Fatalf("an unfinished entry: done %v, %q", m.Done(), line(m))
	}
	if !strings.Contains(m.View(), "open: (") {
		t.Errorf("why not shown: %q", m.View())
	}
	typeInto(m, "<enter>") // an empty line: run it as it is, its error to say why
	if got, _ := m.Result(); !m.Done() || got != "select(.a" {
		t.Errorf("enter on an empty line: done %v, %q", m.Done(), got)
	}

	m = New("> ")
	m.Continues = func(string) string { return "" }
	typeInto(m, ".a`<enter>")
	if m.Done() || line(m) != ".a\n|" {
		t.Errorf("a trailing `: done %v, %q", m.Done(), line(m))
	}
	typeInto(m, "| .b<enter>")
	if got, _ := m.Result(); !m.Done() || got != ".a\n| .b" {
		t.Errorf("then enter: done %v, %q", m.Done(), got)
	}
}

// pasted is a bracketed paste of s.
func pasted(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}
