// Package tui is the interactive viewer: a bubbletea model over a tree.Tree.
//
// The model is pure: Update takes a message and returns the next model, with no terminal.
// Tests drive it with key messages (model_test.go); the view is checked separately.
package tui

import (
	"encoding/json"
	"fmt"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/find"
	"github.com/TimelordUK/jqv/internal/tree"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the viewer's state.
type Model struct {
	Title string // shown in the header: the file name, or "stdin"

	tree   *tree.Tree
	cursor int // row index in tree.Rows()
	top    int // first row on screen
	width  int
	height int

	help    bool   // the help overlay is showing
	pending string // first key of a two-key chord ("z", "g"), or ""
	status  string // one-shot message, cleared by the next key

	// Search. While typing after / or ?, prompt is "/" or "?" and input holds the text.
	prompt  string
	input   string
	query   find.Query  // the last search, highlighted until esc
	forward bool        // direction of the last search; n repeats it, N reverses it
	matches []*doc.Node // every match of query, in document order
	matchAt map[*doc.Node]int

	// Copy puts text on the clipboard. Tests replace it.
	Copy func(string) error
}

// New builds a viewer over a parsed document.
func New(title string, root *doc.Node) Model {
	return Model{
		Title:  title,
		tree:   tree.New(root),
		width:  80,
		height: 24,
		Copy:   func(string) error { return fmt.Errorf("no clipboard") },
	}
}

// Tree exposes the view state, for tests and the --print path.
func (m Model) Tree() *tree.Tree { return m.tree }

// Cursor is the node under the cursor.
func (m Model) Cursor() *doc.Node { return m.tree.Rows()[m.cursor].Node }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
	case tea.KeyMsg:
		if m.prompt != "" {
			m.typing(msg)
			return m, nil
		}
		return m.key(msg.String())
	}
	return m, nil
}

// bodyHeight is the number of tree rows on screen: everything but the header and status bar.
func (m Model) bodyHeight() int { return max(1, m.height-2) }

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	m.status = ""
	if m.help {
		switch k {
		case "f1", "esc", "q":
			m.help = false
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	if p := m.pending; p != "" {
		m.pending = ""
		m.chord(p + k)
		m.scroll()
		return m, nil
	}

	n := m.Cursor()
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d", "pgdown":
		m.move(max(1, m.bodyHeight()/2))
	case "ctrl+u", "pgup":
		m.move(-max(1, m.bodyHeight()/2))
	case "G", "end":
		m.cursor = m.tree.Len() - 1
	case "home":
		m.cursor = 0
	case "g", "z":
		m.pending = k
	case "f1":
		m.help = true
	case "/", "?":
		m.prompt, m.input = k, ""
	case "n":
		m.jump(m.forward)
	case "N":
		m.jump(!m.forward)
	case "esc": // vim's :nohlsearch
		m.query, m.matches, m.matchAt = find.Query{}, nil, nil
	case "l", "right":
		switch {
		case n.IsContainer() && !m.tree.Expanded(n):
			m.tree.SetExpanded(n, true)
		case len(n.Children) > 0:
			m.move(1)
		}
	case "h", "left":
		switch {
		case n.IsContainer() && m.tree.Expanded(n) && n.Parent != nil:
			m.tree.SetExpanded(n, false)
		case n.Parent != nil:
			m.cursor = m.tree.IndexOf(n.Parent)
		}
	case "L": // open everything under the cursor; at the root, the whole document
		m.tree.ExpandAll(n)
	case "H": // close everything under the cursor, keep it open as an outline
		m.tree.CollapseBelow(n)
	case "enter", " ":
		if n.Parent != nil || !m.tree.Expanded(n) {
			m.tree.Toggle(n)
		}
	case "y":
		m.yank(n.Path(), "path")
	case "Y":
		b, err := json.MarshalIndent(n.Value(), "", "  ")
		if err != nil {
			m.status = err.Error()
			break
		}
		m.yank(string(b), "value")
	}
	m.scroll()
	return m, nil
}

// chord handles two-key commands, vim style.
func (m *Model) chord(c string) {
	n := m.Cursor()
	switch c {
	case "gg":
		m.cursor = 0
	case "zz":
		m.top = m.cursor - m.bodyHeight()/2
	case "zt":
		m.top = m.cursor
	case "zb":
		m.top = m.cursor - m.bodyHeight() + 1
	case "zo": // open this node and everything under it
		m.tree.ExpandAll(n)
	case "zc": // close this node and everything under it
		m.tree.CollapseAll(n)
		m.follow(n)
	case "zR":
		m.tree.ExpandAll(m.tree.Root)
		m.follow(n)
	case "zM":
		m.tree.CollapseAll(m.tree.Root)
		m.follow(n)
	default:
		if len(c) == 2 && c[0] == 'z' && c[1] >= '1' && c[1] <= '9' {
			m.tree.ExpandToDepth(int(c[1] - '0'))
			m.follow(n)
		}
	}
}

// follow keeps the cursor on n after a collapse, or on its nearest visible ancestor.
func (m *Model) follow(n *doc.Node) {
	for ; n != nil; n = n.Parent {
		if i := m.tree.IndexOf(n); i >= 0 {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m *Model) move(d int) {
	m.cursor = min(max(m.cursor+d, 0), m.tree.Len()-1)
}

// typing edits the search prompt. Enter searches; esc cancels; an empty pattern repeats the last.
func (m *Model) typing(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEnter:
		if m.input != "" {
			m.query = find.Parse(m.input)
			m.matches = find.All(m.tree.Root, m.query)
			m.matchAt = make(map[*doc.Node]int, len(m.matches))
			for i, n := range m.matches {
				m.matchAt[n] = i
			}
		}
		m.forward = m.prompt == "/"
		m.prompt = ""
		m.jump(m.forward)
	case tea.KeyEsc, tea.KeyCtrlC:
		m.prompt = ""
	case tea.KeyBackspace:
		if r := []rune(m.input); len(r) > 0 {
			m.input = string(r[:len(r)-1])
		} else {
			m.prompt = "" // backspace on an empty prompt leaves it, as in vim
		}
	case tea.KeyCtrlU:
		m.input = ""
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(msg.Runes)
	}
}

// jump moves to the next match in a direction, opening parents to show it.
func (m *Model) jump(forward bool) {
	if m.query.Empty() {
		m.status = "no previous search"
		return
	}
	i, wrapped := find.Next(m.matches, m.Cursor(), forward)
	if i < 0 {
		m.status = "pattern not found: " + m.query.Text
		return
	}
	m.land(m.matches[i])
	switch {
	case wrapped && forward:
		m.status = "search hit BOTTOM, continuing at TOP"
	case wrapped:
		m.status = "search hit TOP, continuing at BOTTOM"
	}
}

// land puts the cursor on n, opening its parents. A target that was off screen is centred,
// as vim does after a search.
func (m *Model) land(n *doc.Node) {
	m.cursor = m.tree.Reveal(n)
	if h := m.bodyHeight(); m.cursor < m.top || m.cursor >= m.top+h {
		m.top = m.cursor - h/2
	}
}

// scroll keeps the cursor on screen.
func (m *Model) scroll() {
	h := m.bodyHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	m.top = max(0, min(m.top, m.tree.Len()-h))
}

func (m *Model) yank(s, what string) {
	if err := m.Copy(s); err != nil {
		m.status = "copy failed: " + err.Error()
		return
	}
	if what == "path" {
		m.status = "copied " + s
	} else {
		m.status = "copied value"
	}
}
