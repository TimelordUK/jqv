package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// printed is what a command prints above the editor (tea.Println), or "".
func printed(c tea.Cmd) string {
	if c == nil {
		return ""
	}
	return strings.Trim(fmt.Sprint(c()), "{}") // a printLineMessage, unexported: its one field
}

// F12 prints what the editor knows (owner 2026-11-21): the terminal's size and whether one
// came at all, the entry's lines and rows, the window, the cursor, and the host's own part.
func TestStateSaysWhatTheEditorKnows(t *testing.T) {
	m := New("> ")
	m.StateOf = func() string { return "session: nothing loaded" }
	m.Update(pasted(numbered(5)))
	cmds := typeInto(m, "<f12>")
	got := printed(cmds[len(cmds)-1])
	for _, want := range []string{"terminal: size unknown", "entry: 5 lines", "cursor: line 5 col 3", "window: whole", "session: nothing loaded"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m.Update(pasted("\n" + numbered(30)))
	cmds = typeInto(m, "<f12>")
	got = printed(cmds[len(cmds)-1])
	for _, want := range []string{"terminal: 80×10", "entry: 35 lines", "window: lines 30-35 of 35, 29 above, 0 below, 6 rows for text"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if m.Text() != numbered(5)+"\n"+numbered(30) {
		t.Errorf("F12 changed the entry")
	}
}

// PgUp and PgDn move the cursor a window's height, so a long entry reads like a pager.
func TestPageUpAndDown(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10}) // 6 rows of text
	m.Update(pasted(numbered(30)))
	typeInto(m, "<pgup>")
	// a page up, the window with it: the cursor keeps its row (l23, at its end)
	if got := shown(m); !strings.HasPrefix(got, "> ↑ 18 more\n… l18\n") || !strings.Contains(got, "… l23 \n") {
		t.Errorf("pgup:\n%s", got)
	}
	typeInto(m, "<pgup><pgup><pgup><pgup><pgup>") // stops at the first line, not history
	if !strings.HasPrefix(line(m), "l0|\n") {
		t.Errorf("pgup to the top: %q", line(m)[:6])
	}
	typeInto(m, "<pgdown>")
	if got := shown(m); !strings.Contains(got, "… l6") {
		t.Errorf("pgdown:\n%s", got)
	}
}
