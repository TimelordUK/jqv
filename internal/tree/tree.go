// Package tree is the expand/collapse state over a document and the flat list of rows it shows.
//
// Only expanded containers contribute their children, so the row list is what is on screen
// (once scrolled) and its length is the scroll range. It has no terminal dependency, so every
// navigation rule is tested here, not through the TUI.
package tree

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Row is one visible line: a node at a depth. Closing brackets are not rows.
type Row struct {
	Node  *doc.Node
	Depth int
}

// Tree is the view state for one document.
type Tree struct {
	Root     *doc.Node
	expanded map[*doc.Node]bool
	rows     []Row
}

// New opens the root and its direct children's level (depth 1), as jless does.
func New(root *doc.Node) *Tree {
	t := &Tree{Root: root, expanded: map[*doc.Node]bool{}}
	t.expanded[root] = true
	t.rebuild()
	return t
}

func (t *Tree) Rows() []Row { return t.rows }
func (t *Tree) Len() int    { return len(t.rows) }

// Expanded reports whether a container shows its children.
func (t *Tree) Expanded(n *doc.Node) bool { return t.expanded[n] }

// IndexOf returns the row showing n, or -1 when it is hidden inside a collapsed parent.
func (t *Tree) IndexOf(n *doc.Node) int {
	for i, r := range t.rows {
		if r.Node == n {
			return i
		}
	}
	return -1
}

func (t *Tree) rebuild() {
	t.rows = t.rows[:0]
	var add func(n *doc.Node, depth int)
	add = func(n *doc.Node, depth int) {
		t.rows = append(t.rows, Row{n, depth})
		if t.expanded[n] {
			for _, c := range n.Children {
				add(c, depth+1)
			}
		}
	}
	add(t.Root, 0)
}

// SetExpanded opens or closes one container. Scalars are ignored.
func (t *Tree) SetExpanded(n *doc.Node, open bool) {
	if !n.IsContainer() {
		return
	}
	if open {
		t.expanded[n] = true
	} else {
		delete(t.expanded, n)
	}
	t.rebuild()
}

// Toggle flips one container.
func (t *Tree) Toggle(n *doc.Node) { t.SetExpanded(n, !t.expanded[n]) }

// ExpandAll opens n and everything below it.
func (t *Tree) ExpandAll(n *doc.Node) {
	n.Walk(func(c *doc.Node) bool {
		if c.IsContainer() {
			t.expanded[c] = true
		}
		return true
	})
	t.rebuild()
}

// CollapseAll closes everything below n, and n itself unless it is the root,
// so the root never disappears into one line.
func (t *Tree) CollapseAll(n *doc.Node) {
	n.Walk(func(c *doc.Node) bool {
		delete(t.expanded, c)
		return true
	})
	if n == t.Root {
		t.expanded[n] = true
	}
	t.rebuild()
}

// CollapseBelow closes every container under n but leaves n open, so its children show as an outline.
func (t *Tree) CollapseBelow(n *doc.Node) {
	for _, c := range n.Children {
		c.Walk(func(d *doc.Node) bool {
			delete(t.expanded, d)
			return true
		})
	}
	if n.IsContainer() {
		t.expanded[n] = true
	}
	t.rebuild()
}

// ExpandToDepth opens every container shallower than depth (root is depth 0) and closes the rest.
func (t *Tree) ExpandToDepth(depth int) {
	t.expanded = map[*doc.Node]bool{t.Root: true}
	var walk func(n *doc.Node, d int)
	walk = func(n *doc.Node, d int) {
		if d >= depth || !n.IsContainer() {
			return
		}
		t.expanded[n] = true
		for _, c := range n.Children {
			walk(c, d+1)
		}
	}
	walk(t.Root, 0)
	t.rebuild()
}

// Label is the plain text of a row, without indentation or colour:
//
//	▾ items [3]
//	▸ meta {4}
//	  qty: 2
//
// The TUI styles the same pieces; this form is what the golden tests pin.
func (t *Tree) Label(r Row) string {
	var b strings.Builder
	n := r.Node
	switch {
	case !n.IsContainer():
		b.WriteString("  ")
	case t.expanded[n]:
		b.WriteString("▾ ")
	default:
		b.WriteString("▸ ")
	}
	if name := Name(n); name != "" {
		b.WriteString(name)
		if !n.IsContainer() {
			b.WriteString(":")
		}
		b.WriteString(" ")
	}
	if n.IsContainer() {
		b.WriteString(Summary(n))
	} else {
		b.WriteString(Scalar(n))
	}
	return b.String()
}

// Name is how a row names its node: the key, `[i]` for an array element, empty for the root.
func Name(n *doc.Node) string {
	switch {
	case n.Parent == nil:
		return ""
	case n.Parent.Kind == doc.Array:
		return "[" + strconv.Itoa(n.Index) + "]"
	default:
		return n.Key
	}
}

// Summary is a container's size: `{4}` or `[120]`.
func Summary(n *doc.Node) string {
	c := strconv.Itoa(len(n.Children))
	if n.Kind == doc.Object {
		return "{" + c + "}"
	}
	return "[" + c + "]"
}

// Scalar renders a scalar as JSON text; strings are quoted and escaped.
func Scalar(n *doc.Node) string {
	if n.Kind == doc.String {
		q, _ := json.Marshal(n.Raw)
		return string(q)
	}
	return n.Raw
}
