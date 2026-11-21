package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// numbered is n lines, l0 to l(n-1).
func numbered(n int) string {
	ls := make([]string, n)
	for i := range ls {
		ls[i] = fmt.Sprintf("l%d", i)
	}
	return strings.Join(ls, "\n")
}

// shown is the view as plain text, the cursor's reverse dropped.
func shown(m *Model) string { return ansi.Strip(m.View()) }

// An entry taller than the terminal shows a window of its lines around the cursor (Q-049,
// DESIGN §23), with how many are hidden above and below; moving past an edge scrolls it.
func TestWindowFollowsTheCursor(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10}) // 6 rows of text, 4 kept for markers and hint
	m.Update(pasted(numbered(30)))
	want := "> ↑ 24 more\n… l24\n… l25\n… l26\n… l27\n… l28\n… l29 "
	if got := shown(m); got != want {
		t.Fatalf("at the end:\n%s\nwant\n%s", got, want)
	}
	typeInto(m, "<up><up><up><up><up>")                             // still inside the window: it stays put
	want = "> ↑ 24 more\n… l24 \n… l25\n… l26\n… l27\n… l28\n… l29" // the cursor at l24's end, a reversed space
	if got := shown(m); got != want {
		t.Errorf("up inside the window moved it:\n%s", got)
	}
	typeInto(m, "<up><up>") // past its top: it scrolls by the lines moved
	want = "> ↑ 22 more\n… l22 \n… l23\n… l24\n… l25\n… l26\n… l27\n  ↓ 2 more"
	if got := shown(m); got != want {
		t.Errorf("past the top:\n%s\nwant\n%s", got, want)
	}
	for range 30 {
		typeInto(m, "<up>")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft}) // off the history walk's end, so up stays in the entry
	want = "> l0\n… l1\n… l2\n… l3\n… l4\n… l5\n  ↓ 24 more"
	if got := shown(m); got != want {
		t.Errorf("at the top:\n%s\nwant\n%s", got, want)
	}
}

// An entry that fits has no window, and a terminal of unknown height never windows.
func TestWindowOnlyWhenTooTall(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m.Update(pasted(numbered(8)))
	if got := shown(m); strings.Contains(got, "more") {
		t.Errorf("8 lines in 10 rows windowed:\n%s", got)
	}
	m = New("> ")
	m.Update(pasted(numbered(100)))
	if got := shown(m); strings.Contains(got, "more") || strings.Count(got, "\n") != 99 {
		t.Errorf("unknown height windowed")
	}
}

// A wrapped line takes the rows it wraps to, so the window still fits the terminal.
func TestWindowCountsWrappedRows(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m.Update(pasted(numbered(20) + "\n" + strings.Repeat("x", 50))) // 3 rows at 20 wide
	got := shown(m)
	if n := strings.Count(got, "\n") + 1; n > 8 {
		t.Errorf("%d rows in a 10-row terminal:\n%s", n, got)
	}
	if !strings.HasPrefix(got, "> ↑ 17 more\n… l17\n") {
		t.Errorf("want l17 to l19 and the long line:\n%s", got)
	}
}

// A finished entry keeps the window it had, so what is printed fits the terminal.
func TestWindowWhenDone(t *testing.T) {
	m := New("> ")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m.Update(pasted(numbered(30)))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.Done() {
		t.Fatal("not done")
	}
	if got := shown(m); !strings.HasPrefix(got, "> ↑ 24 more\n… l24\n") || strings.Count(got, "\n") != 7 {
		t.Errorf("done:\n%q", got)
	}
}
