package tui

import (
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Model is the workbench (DESIGN §25): the Workspace, which owns the state, and the panes that
// draw it. With nothing picked it is the pager, as jqv always was.
type Model struct {
	ws     *Workspace
	width  int
	height int

	layout layout
	focus  focus
	zoom   bool   // F9: the focused pane has the whole screen
	shown  string // the program the panes were last arranged for

	treePane // the document tree; embedded, so its state and keys read as the model's
	table    tablePane
}

type layout uint8

const (
	readLayout layout = iota // the tree alone, as today
	pickLayout               // the tree beside the program and its table
)

type focus uint8

const (
	focusTree focus = iota
	focusTable
)

// sideBySide is the width from which Pick puts the table beside the tree, not under it.
const sideBySide = 120

// New builds a viewer over a parsed document.
func New(title string, root *doc.Node) Model {
	ws := NewWorkspace(title, root)
	m := Model{ws: ws, treePane: newTreePane(ws), table: tablePane{ws: ws}}
	m.resize(80, 24)
	return m
}

// Workspace is the state the panes draw from.
func (m Model) Workspace() *Workspace { return m.ws }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case ranMsg:
		if m.ws.ran(msg) {
			m.table.refresh(m.ws.Columns())
		}
	case tea.KeyMsg:
		cmd = m.key(msg)
		m.follow()
		m.arrange()
	}
	return m, cmd
}

// key handles the workbench's own keys, and hands the rest to the pane with focus. The tree's
// prompt and the help take every key while they are open.
func (m *Model) key(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	if m.prompt == "" && !m.help {
		switch k {
		case "f6", "f7", "f9":
			m.status = ""
			if m.ws.Program() == "" {
				m.status = "nothing picked yet: space on a field in a record picks it"
				return nil
			}
			switch k {
			case "f6":
				if m.layout == pickLayout {
					m.focus = 1 - m.focus
				}
			case "f7": // show or hide the table; the program stays
				m.layout = 1 - m.layout
				m.focus, m.zoom = focusTree, false
			case "f9":
				if m.layout == pickLayout {
					m.zoom = !m.zoom
				}
			}
			return nil
		}
		if m.layout == pickLayout && m.focus == focusTable && k != "f1" {
			m.status = ""
			return m.table.key(k)
		}
	}
	return m.treePane.update(msg)
}

// follow lays the workbench out for the program, when a pick has changed it: the first pick
// opens Pick, the last unpick goes back to Read (DESIGN §26), and a new column takes the
// table's cursor.
func (m *Model) follow() {
	p := m.ws.Program()
	if p == m.shown {
		return
	}
	before := m.table.layout
	switch {
	case p == "":
		m.layout, m.focus, m.zoom = readLayout, focusTree, false
	case m.shown == "":
		m.layout = pickLayout
	}
	m.shown = p
	m.table.refresh(m.ws.Columns())
	if cols := m.ws.Columns(); before != nil && len(cols) > 0 {
		m.table.want = cols[len(cols)-1]
	}
}

// resize takes the terminal's size.
func (m *Model) resize(w, h int) {
	m.width, m.height = w, h
	m.arrange()
}

// arrange gives each pane its share of the screen under the header.
func (m *Model) arrange() {
	h := max(1, m.height-1)
	t, tb := &m.treePane, &m.table
	switch {
	case m.layout == readLayout || m.help || (m.zoom && m.focus == focusTree):
		t.width, t.height = m.width, h
	case m.zoom:
		tb.width, tb.height = m.width, h
	case m.width >= sideBySide:
		tw := m.width * 2 / 5
		t.width, t.height = tw, h
		tb.width, tb.height = m.width-tw-1, h
	default:
		th := max(3, h/3)
		t.width, t.height = m.width, th
		tb.width, tb.height = m.width, max(2, h-th)
	}
	t.scrollPick()
	t.scroll()
	tb.scroll()
}

// View draws the header, then the panes under it.
func (m Model) View() string {
	head := m.header() + "\n"
	switch {
	case m.layout == readLayout || m.help || (m.zoom && m.focus == focusTree):
		return head + m.treePane.view()
	case m.zoom:
		return head + m.table.view()
	case m.width >= sideBySide:
		left := strings.Split(m.treePane.view(), "\n")
		right := strings.Split(m.table.view(), "\n")
		sep := styleGuide.Render("│")
		var b strings.Builder
		for i := range max(len(left), len(right)) {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(l + strings.Repeat(" ", max(0, m.treePane.width-ansi.StringWidth(l))) + sep + r)
		}
		return head + b.String()
	}
	return head + m.treePane.view() + "\n" + m.table.view()
}
