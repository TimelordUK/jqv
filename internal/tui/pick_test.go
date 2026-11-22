package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// do presses keys as press does, and delivers what each run gives back, as bubbletea would.
func do(m Model, keys string) Model {
	for _, k := range strings.Fields(keys) {
		next, cmd := m.Update(key(k))
		m = next.(Model)
		if cmd != nil {
			if msg, ok := cmd().(ranMsg); ok {
				m = send(m, msg)
			}
		}
	}
	return m
}

// onRecord opens the fourth run and puts the cursor on its first field (Project).
func onRecord(t *testing.T) Model {
	t.Helper()
	m := press(teamcity(t), "j j j j l j")
	if got := m.Cursor().Path(); got != ".[3].Project" {
		t.Fatalf("cursor at %s", got)
	}
	return m
}

// DESIGN §26: space on a field picks it; Pick opens by itself, the bar shows the jq, the
// table has a row per record; the last unpick goes back to Read.
func TestPickOpensAndCloses(t *testing.T) {
	m := do(onRecord(t), "j space")
	if m.layout != pickLayout || m.ws.Program() != ".[] | {Job}" {
		t.Fatalf("layout %d, program %q", m.layout, m.ws.Program())
	}
	n := len(m.ws.Doc().Root.Children)
	if m.table.rows() != n || len(m.table.columns()) != 1 {
		t.Fatalf("table %d rows %d columns, want %d and 1", m.table.rows(), len(m.table.columns()), n)
	}
	m = do(m, "j j j space j space") // Status, State (past JobId, Number)
	if got := m.ws.Program(); got != ".[] | {Job, Status, State}" {
		t.Fatalf("program %q", got)
	}
	if c := m.table.columns()[m.table.col]; c.Name != "State" {
		t.Errorf("table cursor on %s, want the column just picked", c.Name)
	}
	m = do(m, "k space k k k space j j j j space") // unpick Status, Job, State
	if m.layout != readLayout || m.ws.Program() != "" {
		t.Errorf("all unpicked: layout %d, program %q", m.layout, m.ws.Program())
	}
	if m.focus != focusTree {
		t.Error("focus left on a table that isn't there")
	}
}

// A field picked in one record is marked in every record.
func TestPickMarksEveryRecord(t *testing.T) {
	m := do(onRecord(t), "j space")
	m = press(m, "k k k space") // up to [2], toggling it open is not a pick
	found := false
	for _, r := range m.Tree().Rows() {
		if r.Node.Path() == ".[2].Job" {
			found = m.picked(r.Node)
		}
	}
	if !found {
		t.Error(".[2].Job is not marked picked")
	}
	if !strings.Contains(m.View(), "● Job:") {
		t.Errorf("no ● in the view:\n%s", m.View())
	}
}

// A field that can't be picked says why, and nothing changes.
func TestPickRefused(t *testing.T) {
	m := do(onRecord(t), "k space") // the record itself: a container toggles
	if m.ws.Program() != "" {
		t.Fatal("space on a record picked it")
	}
	if m.Tree().Expanded(m.Cursor()) {
		t.Error("space on an open record should close it, as before")
	}
	m = do(teamcity(t), "f6")
	if !strings.Contains(m.status, "nothing picked yet") {
		t.Errorf("f6 with nothing picked: %q", m.status)
	}
}

// F6 moves focus to the table, where keys move its cursor and not the tree's.
func TestPickTableKeys(t *testing.T) {
	m := do(onRecord(t), "j space j j space f6")
	if m.focus != focusTable {
		t.Fatal("f6 didn't move focus")
	}
	cur := m.cursor
	m = do(m, "j j h")
	if m.table.row != 2 || m.table.col != 0 || m.cursor != cur {
		t.Errorf("table row %d col %d, tree cursor moved %v", m.table.row, m.table.col, m.cursor != cur)
	}
	if want := "Job: " + m.table.layout.Full(2, 0); !strings.Contains(m.View(), want) {
		t.Errorf("status line should show %q:\n%s", want, m.View())
	}
	m = do(m, "f6 j")
	if m.focus != focusTree || m.cursor != cur+1 {
		t.Error("f6 back to the tree")
	}
}

// F7 hides the table and keeps the program; F9 zooms the pane with focus.
func TestPickLayoutKeys(t *testing.T) {
	m := do(onRecord(t), "j space")
	m = do(m, "f7")
	if m.layout != readLayout || m.ws.Program() == "" {
		t.Fatalf("f7: layout %d, program %q", m.layout, m.ws.Program())
	}
	m = do(m, "f7 f6 f9")
	if !m.zoom || m.table.height != m.height-1 || m.table.width != m.width {
		t.Errorf("zoom: table %dx%d in %dx%d", m.table.width, m.table.height, m.width, m.height)
	}
}

// Every layout fills the screen exactly, and no line is wider than it.
func TestPickViewFits(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 140, Height: 30}, {Width: 80, Height: 24}, {Width: 40, Height: 10}} {
		for _, keys := range []string{"", "f6 f9", "f7"} {
			m := do(send(onRecord(t), size), "j space j j space j space "+keys)
			v := m.View()
			lines := strings.Split(v, "\n")
			if len(lines) != size.Height {
				t.Errorf("%dx%d %q: %d lines", size.Width, size.Height, keys, len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > size.Width {
					t.Errorf("%dx%d %q: line %d is %d wide: %q", size.Width, size.Height, keys, i, w, ansi.Strip(l))
				}
			}
		}
	}
}

// Side by side from 120 columns, the bar beside the tree; stacked below.
func TestPickSideBySide(t *testing.T) {
	m := do(send(onRecord(t), tea.WindowSizeMsg{Width: 140, Height: 30}), "j space")
	if m.treePane.width+1+m.table.width != 140 || m.table.height != 29 {
		t.Errorf("side by side: tree %d, table %dx%d", m.treePane.width, m.table.width, m.table.height)
	}
	second := ansi.Strip(strings.Split(m.View(), "\n")[1])
	if !strings.Contains(second, "│ .[] | {Job}") {
		t.Errorf("the bar should be beside the tree: %q", second)
	}
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.treePane.width != 100 || m.table.width != 100 || m.treePane.height+m.table.height != 29 {
		t.Errorf("stacked: tree %dx%d, table %dx%d", m.treePane.width, m.treePane.height, m.table.width, m.table.height)
	}
}
