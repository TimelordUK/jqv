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
	"github.com/TimelordUK/jqv/internal/pick"
	"github.com/TimelordUK/jqv/internal/tree"
	tea "github.com/charmbracelet/bubbletea"
)

// finderPrompt is the prompt of the finder, which also tells it from / ? and :.
const finderPrompt = "> "

// treePane is the document as a tree: the pager, with search, the finder and :set.
type treePane struct {
	ws   *Workspace // asked for changes: a pick sets the program
	doc  *Document  // what it shows
	tree *tree.Tree
	// The document's tree as it was left while tree shows what a stage gets (DESIGN §28.2).
	docTree           *tree.Tree
	docCursor, docTop int
	cursor            int // row index in tree.Rows()
	top               int // first row on screen
	width             int
	height            int

	nodes   int       // values in the whole document, for ctrl+g
	number  bool      // :set nu, row numbers in a gutter
	relnum  bool      // :set rnu, numbers relative to the cursor
	lock    bool      // zl, the cursor stays on the middle row
	depth   int       // :set depth, how far the document opened at the start
	help    bool      // the help overlay is showing
	pending string    // first key of a two-key chord ("z", "g"), or ""
	status  string    // one-shot message, cleared by the next key
	matched *doc.Node // the block % or ]} last went to the end of; % at that end returns to it

	// The prompt line. While typing after /, ? or :, prompt is that character and input the text.
	prompt  string
	input   string
	query   find.Query  // the last search, highlighted until esc; live while one is being typed
	forward bool        // direction of the last search; n repeats it, N reverses it
	matches []*doc.Node // every match of query, in document order

	before  undo // what esc goes back to while a prompt is open
	pick    int  // the finder's selected hit, an index into matches
	pickTop int  // first hit on screen in the finder's list

	configPath string // where config.toml was looked for, for the help

	// Copy puts text on the clipboard. Tests replace it.
	Copy func(string) error
}

// undo is the state from before a prompt opened, plus what its preview has opened since.
type undo struct {
	cursor, top int
	query       find.Query
	matches     []*doc.Node
	forward     bool
	opened      []*doc.Node
}

func newTreePane(ws *Workspace) treePane {
	d := ws.Doc()
	nodes := 0
	d.Root.Walk(func(*doc.Node) bool { nodes++; return true })
	return treePane{
		ws:    ws,
		doc:   d,
		nodes: nodes,
		tree:  tree.New(d.Root),
		depth: 1,
		Copy:  func(string) error { return fmt.Errorf("no clipboard") },
	}
}

// Tree exposes the view state, for tests and the --print path.
func (m treePane) Tree() *tree.Tree { return m.tree }

// Cursor is the node under the cursor.
func (m treePane) Cursor() *doc.Node { return m.tree.Rows()[m.cursor].Node }

// update is a key for the tree.
func (m *treePane) update(msg tea.KeyMsg) tea.Cmd {
	if m.prompt != "" {
		m.status = ""
		cmd := m.typing(msg)
		if m.prompt == "" {
			m.input = ""
		}
		return cmd
	}
	return m.key(msg.String())
}

// bodyHeight is the number of tree rows on screen: the pane but its status bar and the
// finder's list with the rule above it.
func (m treePane) bodyHeight() int {
	h := max(1, m.height-1)
	if l := m.listRows(); l > 0 {
		h -= l + 1
	}
	return h
}

// finding reports whether the finder is open.
func (m treePane) finding() bool { return m.prompt == finderPrompt }

// listRows is the height of the finder's list, 0 when it is closed or there is no room for it.
// It does not change with the number of hits, so the tree does not jump about during typing.
func (m treePane) listRows() int {
	if h := m.height - 1; m.finding() && h >= 5 {
		return h * 2 / 5
	}
	return 0
}

func (m *treePane) key(k string) tea.Cmd {
	m.status = ""
	if m.help {
		switch k {
		case "f1", "esc", "q":
			m.help = false
		case "ctrl+c":
			return tea.Quit
		}
		return nil
	}
	if p := m.pending; p != "" {
		m.pending = ""
		m.chord(p + k)
		m.scroll()
		return nil
	}

	n := m.Cursor()
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
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
	case ":", "/", "?":
		m.open(k, "")
	case "f":
		m.open(finderPrompt, "")
	case "F":
		m.reopen()
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
	case "n":
		m.jump(m.forward)
	case "N":
		m.jump(!m.forward)
	case "esc": // vim's :nohlsearch
		m.query, m.matches = find.Query{}, nil
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
	case " ":
		if !n.IsContainer() {
			m.scroll()
			return m.pickField(n) // DESIGN §26: space on a field picks it
		}
		if n.Parent != nil || !m.tree.Expanded(n) {
			m.tree.Toggle(n)
		}
	case "enter":
		if n.Parent != nil || !m.tree.Expanded(n) {
			m.tree.Toggle(n)
		}
	case "=", "|":
		return m.where(n, plain(pick.Eq), k == "|") // DESIGN §28.3, §29: a filter on this value, | joining with or
	case "!":
		return m.where(n, plain(pick.Ne), false) // Q-071: anything but this value
	case "<", ">":
		return m.where(n, func(ks []string, n *doc.Node) (string, error) { return pick.Compare(ks, n, k) }, false) // Q-072
	case "x":
		m.scroll()
		return m.join(n) // DESIGN §29: a cross join to this array
	case "y":
		m.yank(n.Path(), "path")
	case "Y":
		if n.Kind == doc.String { // pasted as text, so no quotes or escapes
			m.yank(n.Raw, "value")
			break
		}
		b, err := json.MarshalIndent(n.Value(), "", "  ")
		if err != nil {
			m.status = err.Error()
			break
		}
		m.yank(string(b), "value")
	}
	m.scroll()
	return nil
}

// chord handles two-key commands, vim style.
func (m *treePane) chord(c string) {
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
func (m *treePane) enclosing(closing bool) {
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
func (m *treePane) matchBlock() {
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
func (m *treePane) sibling(dir int) {
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
func (m *treePane) element(dir int) {
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
func (m *treePane) arrive(n *doc.Node) {
	m.tree.SetExpanded(n, true)
	m.cursor = m.tree.Reveal(n)
	m.top = m.cursor
}

// follow keeps the cursor on n after a collapse, or on its nearest visible ancestor.
func (m *treePane) follow(n *doc.Node) {
	for ; n != nil; n = n.Parent {
		if i := m.tree.IndexOf(n); i >= 0 {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m *treePane) move(d int) {
	m.cursor = min(max(m.cursor+d, 0), m.tree.Len()-1)
}

// page scrolls a screen, as vim's ctrl+f and ctrl+b, keeping two rows of context, and takes
// the cursor with it.
func (m *treePane) page(dir int) {
	step := max(1, m.bodyHeight()-2) * dir
	m.top = max(0, min(m.top+step, m.tree.Len()-m.bodyHeight()))
	m.move(step)
	if dir > 0 && m.cursor < m.top {
		m.cursor = m.top
	}
}

// position is ctrl+g's report: where the cursor is among the rows on show, and how far through
// the whole document, which does not change as nodes open and close.
func (m treePane) position() string {
	n := m.Cursor()
	depth := m.tree.Rows()[m.cursor].Depth
	return fmt.Sprintf("%q  row %d of %d --%d%%--  value %d of %d (%d%%)  depth %d",
		m.doc.Name, m.cursor+1, m.tree.Len(), percent(m.cursor+1, m.tree.Len()),
		n.Seq+1, m.nodes, percent(n.Seq+1, m.nodes), depth)
}

func percent(a, b int) int { return a * 100 / max(1, b) }

// command runs a : command line. Only a few of vim's: a row number, :set (options.go), :q.
func (m *treePane) command(c string) tea.Cmd {
	c = strings.TrimSpace(c)
	if n, err := strconv.Atoi(c); err == nil {
		m.cursor = min(max(n-1, 0), m.tree.Len()-1)
		if h := m.bodyHeight(); m.cursor < m.top || m.cursor >= m.top+h {
			m.top = m.cursor - h/2
		}
		return nil
	}
	verb, args, _ := strings.Cut(c, " ")
	switch verb {
	case "":
	case "q", "q!", "qa", "quit":
		return tea.Quit
	case "se", "set":
		m.set(args)
	default:
		m.status = "not a command: " + c
	}
	return nil
}

// open starts a prompt, remembering what esc must put back.
func (m *treePane) open(prompt, input string) {
	m.prompt, m.input = prompt, input
	m.pick, m.pickTop = 0, 0
	m.before = undo{cursor: m.cursor, top: m.top, query: m.query, matches: m.matches, forward: m.forward}
	if prompt == finderPrompt {
		m.query, m.matches = find.Query{}, nil
	}
}

// reopen shows the last search as the finder's list, on the match at the cursor or the next one
// after it, so a list can be left and picked up again like vim's quickfix.
func (m *treePane) reopen() {
	if m.query.Empty() {
		m.status = "no previous search"
		return
	}
	query, matches, cursor := m.query, m.matches, m.Cursor()
	m.open(finderPrompt, query.Raw)
	m.query, m.matches = query, matches
	if m.pick = find.Index(matches, cursor); m.pick < 0 {
		m.pick, _ = find.Next(matches, cursor, true)
	}
	m.pick = max(m.pick, 0)
	m.scrollPick()
	m.previewPick()
}

// rewind takes back the preview: what it opened, and where the cursor went.
func (m *treePane) rewind() {
	m.tree.Unpeek(m.before.opened)
	m.before.opened = nil
	m.cursor, m.top = m.before.cursor, m.before.top
}

// cancel leaves the prompt with everything as it was before it opened.
func (m *treePane) cancel() {
	m.rewind()
	m.query, m.matches, m.forward = m.before.query, m.before.matches, m.before.forward
	m.prompt = ""
	m.scroll()
}

// refresh follows the pattern as it is typed: the matches, and the cursor on the first of them
// looking from where the prompt was opened. Nothing is decided until enter.
func (m *treePane) refresh() {
	m.rewind()
	q := find.Parse(m.input)
	switch {
	case m.input == "" && m.finding():
		m.query, m.matches = find.Query{}, nil
	case m.input == "":
		m.query, m.matches = m.before.query, m.before.matches
	case q.Refines(m.query):
		m.query, m.matches = q, find.Filter(m.matches, q)
	default:
		m.query, m.matches = q, find.All(m.tree.Root, q)
	}
	if m.finding() {
		m.pick, m.pickTop = 0, 0
		m.previewPick()
	} else if i, _ := find.Next(m.matches, m.Cursor(), m.prompt == "/"); i >= 0 && m.input != "" {
		m.peek(m.matches[i])
	}
	m.scroll()
}

// previewPick shows the selected hit in the tree.
func (m *treePane) previewPick() {
	if m.pick < len(m.matches) {
		m.peek(m.matches[m.pick])
	}
}

// movePick moves the finder's selection, stopping at the ends of the list.
func (m *treePane) movePick(d int) {
	m.rewind()
	m.pick = max(0, min(m.pick+d, len(m.matches)-1))
	m.scrollPick()
	m.previewPick()
	m.scroll()
}

// scrollPick keeps the selection on screen in the finder's list.
func (m *treePane) scrollPick() {
	h := max(1, m.listRows())
	if m.pick < m.pickTop {
		m.pickTop = m.pick
	}
	if m.pick >= m.pickTop+h {
		m.pickTop = m.pick - h + 1
	}
	m.pickTop = max(0, min(m.pickTop, len(m.matches)-h))
}

// typing edits the prompt line. Enter searches; esc cancels; an empty pattern repeats the last.
func (m *treePane) typing(msg tea.KeyMsg) tea.Cmd {
	if m.finding() {
		switch msg.String() {
		case "down", "ctrl+j":
			m.movePick(1)
			return nil
		case "up", "ctrl+k":
			m.movePick(-1)
			return nil
		case "pgdown":
			m.movePick(max(1, m.listRows()))
			return nil
		case "pgup":
			m.movePick(-max(1, m.listRows()))
			return nil
		}
	}
	switch msg.Type {
	case tea.KeyEnter:
		if m.prompt == ":" {
			m.prompt = ""
			cmd := m.command(m.input)
			m.scroll()
			return cmd
		}
		if m.finding() {
			m.pickHit()
			return nil
		}
		m.rewind() // jump lands again, and reports wrapping
		m.forward = m.prompt == "/"
		m.prompt = ""
		m.jump(m.forward)
		m.scroll()
		return nil
	case tea.KeyEsc, tea.KeyCtrlC:
		m.cancel()
		return nil
	case tea.KeyBackspace:
		r := []rune(m.input)
		if len(r) == 0 {
			m.cancel() // backspace on an empty prompt leaves it, as in vim
			return nil
		}
		m.input = string(r[:len(r)-1])
	case tea.KeyCtrlU:
		m.input = ""
	case tea.KeySpace:
		m.input += " "
	case tea.KeyRunes:
		m.input += string(msg.Runes)
	default:
		return nil
	}
	if m.prompt != ":" {
		m.refresh()
	}
	return nil
}

// pickHit closes the finder on the selected hit, which leaves it the last search, so n and N
// carry on through the same list.
func (m *treePane) pickHit() {
	if m.query.Empty() {
		m.cancel()
		return
	}
	m.rewind()
	m.forward = true
	m.prompt = ""
	if len(m.matches) == 0 {
		m.status = "pattern not found: " + m.query.Text
	} else {
		m.land(m.matches[m.pick])
	}
	m.scroll()
}

// jump moves to the next match in a direction, opening parents to show it.
func (m *treePane) jump(forward bool) {
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
func (m *treePane) land(n *doc.Node) { m.place(m.tree.Reveal(n)) }

// peek is land for a preview, which rewind can take back.
func (m *treePane) peek(n *doc.Node) {
	row, opened := m.tree.Peek(n)
	m.before.opened = opened
	m.place(row)
}

func (m *treePane) place(row int) {
	m.cursor = row
	if h := m.bodyHeight(); m.cursor < m.top || m.cursor >= m.top+h {
		m.top = m.cursor - h/2
	}
}

// scroll keeps the cursor on screen, or on the middle row while locked. The ends of the
// document still stop the view, as with vim's scrolloff=999.
func (m *treePane) scroll() {
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

func (m *treePane) yank(s, what string) {
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

// pickField picks the field at n into the program, or unpicks it (Q-013): the program is read
// back as picks, changed, and written again, so nothing is kept beside it (DESIGN §26).
func (m *treePane) pickField(n *doc.Node) tea.Cmd {
	p, err := pick.Lift(n)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if !m.onDocument() {
		m.status = "space picks from the document: K back to the picks"
		return nil
	}
	f, ok := m.ws.Form()
	if !ok {
		m.status = "the program isn't one picks wrote, so space can't change it"
		return nil
	}
	if f, err = pick.Toggle(f, p); err != nil {
		m.status = err.Error()
		return nil
	}
	cmd, err := m.ws.SetPicks(f)
	if err != nil {
		m.status = err.Error()
	}
	return cmd
}

// where filters on n's value (DESIGN §28.3): the tree is what goes into the cursor's stage,
// a record per value, so the condition is on n's field in each record.
func (m *treePane) where(n *doc.Node, cond func([]string, *doc.Node) (string, error), or bool) tea.Cmd {
	if m.onDocument() {
		if m.ws.Program() == "" {
			m.status = "= filters what a stage gives: pick fields first, then J to the end"
		} else {
			m.status = "= on the document waits for inserting stages (R-015): J to the end to filter the rows"
		}
		return nil
	}
	ks, err := pick.Field(n)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	c, err := cond(ks, n)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	cmd, err := m.ws.Where(c, or)
	if err != nil {
		m.status = err.Error()
	}
	return cmd
}

// plain is a condition that can't fail, as where takes one that can.
func plain(f func([]string, *doc.Node) string) func([]string, *doc.Node) (string, error) {
	return func(ks []string, n *doc.Node) (string, error) { return f(ks, n), nil }
}

// onDocument says whether the tree is showing the document, rather than what a stage gets.
func (m treePane) onDocument() bool { return m.tree.Root == m.doc.Root }

// showInput puts what goes into the cursor's stage in the tree (DESIGN §28.2): the document
// at the first stage, else the input's run as a tree. The document's tree is kept as it was
// left, cursor and all, for coming back to; a new input keeps the cursor's row.
func (m *treePane) showInput() {
	want, _ := m.ws.Input()
	switch {
	case m.ws.At() == 0 || len(m.ws.Stages()) == 0:
		want = m.doc.Root
	case want == nil:
		return // not back yet: keep what is on show
	}
	if m.tree.Root == want {
		return
	}
	fromDoc := m.onDocument()
	if fromDoc {
		m.docTree, m.docCursor, m.docTop = m.tree, m.cursor, m.top
	}
	switch {
	case want == m.doc.Root:
		m.tree, m.cursor, m.top = m.docTree, m.docCursor, m.docTop
	default:
		m.tree = tree.New(want)
		m.tree.ExpandToDepth(2)
		if fromDoc {
			m.cursor, m.top = 0, 0
		}
	}
	m.query, m.matches = find.Query{}, nil // a search was over the other tree
	m.cursor = max(0, min(m.cursor, m.tree.Len()-1))
	m.scroll()
}

// picked says whether n is a field the program picks, in any record, for its ● in the tree.
func (m treePane) picked(n *doc.Node) bool {
	f, _ := m.ws.Form()
	if f.Empty() || n.IsContainer() || !m.onDocument() {
		return false
	}
	p, err := pick.Lift(n)
	return err == nil && f.Has(p)
}

// joined says whether n is an array the picks join to (DESIGN §29), in any record.
func (m treePane) joined(n *doc.Node) bool {
	f, _ := m.ws.Form()
	return m.onDocument() && f.Joined(n)
}

// join toggles a cross join to the array at n, or the array n is a value in (x, DESIGN §29).
func (m *treePane) join(n *doc.Node) tea.Cmd {
	if !m.onDocument() {
		m.status = "x joins from the document: K back to the picks"
		return nil
	}
	f, ok := m.ws.Form()
	if !ok {
		m.status = "the program isn't one picks wrote, so x can't change it"
		return nil
	}
	f, err := pick.Join(f, n)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	cmd, err := m.ws.SetPicks(f)
	if err != nil {
		m.status = err.Error()
	}
	return cmd
}
