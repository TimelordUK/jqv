package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A preview (Q-054, DESIGN §24) shows its result under the line, redrawn in place: a second
// step replaces the first, nothing is printed above, and typing clears it.
func TestPreviewReplacesItself(t *testing.T) {
	m := New("> ")
	m.Keys["f5"] = "run-to"
	n := 0
	m.Previews = map[Action]Handler{"run-to": func(line string, cur int) (string, int) {
		n++
		return strings.Repeat("x", n) + "\n(1 output)\n", cur
	}}
	cmds := typeInto(m, ".a<f5>")
	if c := cmds[len(cmds)-1]; c != nil {
		t.Errorf("a preview printed above: %v", printed(c))
	}
	if got := shown(m); got != "> .a \nx\n(1 output)" {
		t.Errorf("first:\n%s", got)
	}
	typeInto(m, "<f5>")
	if got := shown(m); got != "> .a \nxx\n(1 output)" {
		t.Errorf("second replaces the first:\n%s", got)
	}
	typeInto(m, "<left>") // moving keeps it
	if !strings.Contains(shown(m), "xx") {
		t.Errorf("moving the cursor dropped it")
	}
	typeInto(m, "b") // typing changes the line: the preview is stale, so it goes
	if got := shown(m); strings.Contains(got, "xx") {
		t.Errorf("typing kept it:\n%s", got)
	}
}

// A long preview is cut to a few rows with how many more, and the window gives it room.
func TestPreviewIsCut(t *testing.T) {
	m := New("> ")
	m.Keys["f5"] = "run-to"
	m.Previews = map[Action]Handler{"run-to": func(line string, cur int) (string, int) {
		return numbered(50), cur
	}}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.Update(pasted(numbered(40)))
	typeInto(m, "<f5>")
	got := shown(m)
	// a third of the rows less the spare, (30-4)/3 = 8: 7 lines and the count
	if !strings.Contains(got, "\nl0\n") || !strings.Contains(got, "\nl6\n… 43 more lines") {
		t.Errorf("preview:\n%s", got)
	}
	if rows := strings.Count(got, "\n") + 1; rows > 30-2 {
		t.Errorf("%d rows in a 30-row terminal:\n%s", rows, got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(shown(m), "more lines") {
		t.Errorf("the finished line kept the preview")
	}
}
