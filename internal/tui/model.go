// Package tui is the interactive viewer: a bubbletea model over a tree.Tree.
//
// The model is pure: Update takes a message and returns the next model, with no terminal.
// Tests drive it with key messages (model_test.go); the view is checked separately.
package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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

	nodes   int       // values in the whole document, for ctrl+g
	number  bool      // :set nu, row numbers in a gutter
	relnum  bool      // :set rnu, numbers relative to the cursor
	lock    bool      // zl, the cursor stays on the middle row
	help    bool      // the help overlay is showing
	pending string    // first key of a two-key chord ("z", "g"), or ""
	status  string    // one-shot message, cleared by the next key
	matched *doc.Node // the block % or ]} last went to the end of; % at that end returns to it

	// The prompt line. While typing after /, ? or :, prompt is that character and input the text.
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
	nodes := 0
	root.Walk(func(*doc.Node) bool { nodes++; return true })
	return Model{
		Title:  title,
		nodes:  nodes,
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
			m.status = ""
			cmd := m.typing(msg)
			return m, cmd
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
	case "ctrl+d":
		m.move(max(1, m.bodyHeight()/2))
	case "ctrl+u":
		m.move(-max(1, m.bodyHeight()/2))
	case "ctrl+f", "pgdown":
		m.page(1)
	case "ctrl+b", "pgup":
		m.page(-1)
	case "ctrl+g":
		m.status = m.position()
	case ":":
		m.prompt, m.input = k, ""
	case "G", "end":
		m.cursor = m.tree.Len() - 1
	case "home":
		m.cursor = 0
	case "g", "z", "[", "]":
		m.pending = k
	case "%":
		m.matchBlock()
	case "}":
		m.sibling(1)
	case "{":
		m.sibling(-1)
	case ")":
		m.element(1)
	case "(":
		m.element(-1)
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
	case "[{":
		m.enclosing(false)
	case "]}":
		m.enclosing(true)
	case "z/": // collapse everything except the paths down to each match of the last search
		if m.query.Empty() {
			m.status = "no previous search"
			return
		}
		if len(m.matches) == 0 { // nothing to show: leave the tree as it is
			m.status = "pattern not found: " + m.query.Text
			return
		}
		m.tree.ShowOnly(m.matches)
		m.cursor, m.top = 0, 0
		m.land(m.matches[0])
		m.status = fmt.Sprintf("showing %d matches of %s", len(m.matches), m.query.Text)
	case "zz":
		m.top = m.cursor - m.bodyHeight()/2
	case "zl":
		m.lock = !m.lock
	case "zt":
		m.lock = false
		m.top = m.cursor
	case "zb":
		m.lock = false
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

// enclosing goes to the nearest { around the cursor, or its }, as vim's [{ and ]}. Arrays are
// skipped, so it climbs by records. On a block's last row ]} means the next one out.
func (m *Model) enclosing(closing bool) {
	for p := m.Cursor().Parent; p != nil; p = p.Parent {
		if p.Kind != doc.Object {
			continue
		}
		if !closing {
			m.land(p)
			return
		}
		if e := m.tree.End(p); e != m.cursor {
			m.matched = p
			m.land(m.tree.Rows()[e].Node)
			return
		}
	}
	if closing {
		m.status = "no enclosing }"
	} else {
		m.status = "no enclosing {"
	}
}

// matchBlock is %: from the start of a block to its last row, opening it if need be, and back.
// A scalar or an empty block goes to the start of the block it is in. One row can end several
// blocks, so the way back is remembered rather than worked out.
func (m *Model) matchBlock() {
	n := m.Cursor()
	switch {
	case m.matched != nil && m.matched != n && m.tree.End(m.matched) == m.cursor:
		m.land(m.matched)
	case len(n.Children) > 0:
		m.tree.SetExpanded(n, true)
		m.matched = n
		m.land(m.tree.Rows()[m.tree.End(n)].Node)
	case n.Parent != nil:
		m.land(n.Parent)
	}
}

// sibling moves to the next or previous block at this level, skipping scalars, opens it and puts
// it at the top of the screen with its contents below. From a scalar it moves from the block the
// scalar is in, so a search hit on a field can step straight to the next record. Blocks it
// leaves stay open, as they do after a search.
func (m *Model) sibling(dir int) {
	n := m.Cursor()
	if !n.IsContainer() && n.Parent != nil {
		n = n.Parent
	}
	if n.Parent != nil {
		kids := n.Parent.Children
		for i := n.Index + dir; i >= 0 && i < len(kids); i += dir {
			if c := kids[i]; c.IsContainer() {
				m.arrive(c)
				return
			}
		}
	}
	if dir > 0 {
		m.status = "no next block"
	} else {
		m.status = "no previous block"
	}
}

// element moves to the next or previous element of the array the cursor is in, from any depth
// inside the current element, and stops at the ends of that array. On an array's own row it
// goes in: the first element going forward, the last going back.
func (m *Model) element(dir int) {
	n := m.Cursor()
	if n.Kind == doc.Array && len(n.Children) > 0 && (n.Parent == nil || n.Parent.Kind != doc.Array) {
		if dir > 0 {
			m.arrive(n.Children[0])
		} else {
			m.arrive(n.Children[len(n.Children)-1])
		}
		return
	}
	for ; n.Parent != nil; n = n.Parent {
		if n.Parent.Kind != doc.Array {
			continue
		}
		kids := n.Parent.Children
		switch i := n.Index + dir; {
		case i >= len(kids):
			m.status = "last element"
		case i < 0:
			m.status = "first element"
		default:
			m.arrive(kids[i])
		}
		return
	}
	m.status = "not in an array"
}

// arrive puts the cursor on n, opened, at the top of the screen with its contents below.
func (m *Model) arrive(n *doc.Node) {
	m.tree.SetExpanded(n, true)
	m.cursor = m.tree.Reveal(n)
	m.top = m.cursor
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

// page scrolls a screen, as vim's ctrl+f and ctrl+b, keeping two rows of context, and takes
// the cursor with it.
func (m *Model) page(dir int) {
	step := max(1, m.bodyHeight()-2) * dir
	m.top = max(0, min(m.top+step, m.tree.Len()-m.bodyHeight()))
	m.move(step)
	if dir > 0 && m.cursor < m.top {
		m.cursor = m.top
	}
}

// position is ctrl+g's report: where the cursor is among the rows on show, and how far through
// the whole document, which does not change as nodes open and close.
func (m Model) position() string {
	n := m.Cursor()
	depth := m.tree.Rows()[m.cursor].Depth
	return fmt.Sprintf("%q  row %d of %d --%d%%--  value %d of %d (%d%%)  depth %d",
		m.Title, m.cursor+1, m.tree.Len(), percent(m.cursor+1, m.tree.Len()),
		n.Seq+1, m.nodes, percent(n.Seq+1, m.nodes), depth)
}

func percent(a, b int) int { return a * 100 / max(1, b) }

// command runs a : command line. Only a few of vim's: a row number, :set for the gutter, :q.
func (m *Model) command(c string) tea.Cmd {
	c = strings.TrimSpace(c)
	if n, err := strconv.Atoi(c); err == nil {
		m.cursor = min(max(n-1, 0), m.tree.Len()-1)
		if h := m.bodyHeight(); m.cursor < m.top || m.cursor >= m.top+h {
			m.top = m.cursor - h/2
		}
		return nil
	}
	switch c {
	case "":
	case "q", "q!", "qa", "quit":
		return tea.Quit
	case "set nu", "set number":
		m.number = true
	case "set nonu", "set nonumber":
		m.number, m.relnum = false, false
	case "set nu!", "set invnu":
		m.number = !m.number
	case "set rnu", "set relativenumber":
		m.relnum = true
	case "set nornu", "set norelativenumber":
		m.relnum = false
	case "set rnu!", "set invrnu":
		m.relnum = !m.relnum
	default:
		m.status = "not a command: " + c
	}
	return nil
}

// typing edits the prompt line. Enter searches; esc cancels; an empty pattern repeats the last.
func (m *Model) typing(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		if m.prompt == ":" {
			m.prompt = ""
			cmd := m.command(m.input)
			m.scroll()
			return cmd
		}
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
		m.scroll()
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
	return nil
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

// scroll keeps the cursor on screen, or on the middle row while locked. The ends of the
// document still stop the view, as with vim's scrolloff=999.
func (m *Model) scroll() {
	h := m.bodyHeight()
	if m.lock {
		m.top = m.cursor - h/2
	}
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
