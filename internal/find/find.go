// Package find is text search over a document: which nodes match, and which match comes next.
//
// A node matches on its key (object members) or its scalar value. Array indexes and
// container summaries are not searched. Case follows vim's smartcase: an all-lower-case
// pattern ignores case, any capital makes it exact; `\c` or `\C` anywhere forces either.
//
// A pattern in double quotes looks the way a string looks in the tree, and matches a whole key
// or string rather than part of one: `"AE"` finds AE but not TAKE, nor the number or bool a
// bare pattern would. While the closing quote is still to come, as it is during incremental
// search, `"AE` matches keys and strings that start with AE.
package find

import (
	"sort"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Match modes, each stricter than the last.
const (
	contains = iota // anywhere in a key or a scalar
	prefix          // the start of a key or string: `"AE`
	exact           // a whole key or string: `"AE"`
)

// Query is a parsed search pattern.
type Query struct {
	Raw  string // the pattern as typed
	Text string // the pattern, with any \c or \C removed
	Fold bool   // ignore case
	cmp  string // Text, lower-cased when Fold
	mode int
}

// Parse reads a pattern as typed after / or ?.
func Parse(raw string) Query {
	s := raw
	force := 0 // 1: \c ignore case, 2: \C match case
	if strings.Contains(s, `\C`) {
		force = 2
	} else if strings.Contains(s, `\c`) {
		force = 1
	}
	s = strings.NewReplacer(`\c`, "", `\C`, "").Replace(s)
	mode := contains
	if strings.HasPrefix(s, `"`) {
		mode, s = prefix, s[1:]
		if len(s) > 0 && s[len(s)-1] == '"' {
			mode, s = exact, s[:len(s)-1]
		}
	}
	fold := strings.ToLower(s) == s
	switch force {
	case 1:
		fold = true
	case 2:
		fold = false
	}
	q := Query{Raw: raw, Text: s, Fold: fold, cmp: s, mode: mode}
	if fold {
		q.cmp = strings.ToLower(s)
	}
	return q
}

// Empty reports whether there is nothing to look for.
func (q Query) Empty() bool { return q.Text == "" }

func (q Query) has(s string) bool {
	if q.Fold {
		s = strings.ToLower(s)
	}
	switch q.mode {
	case prefix:
		return strings.HasPrefix(s, q.cmp)
	case exact:
		return s == q.cmp
	}
	return strings.Contains(s, q.cmp)
}

// MatchKey reports whether n's key matches. Only object members have keys.
func (q Query) MatchKey(n *doc.Node) bool {
	return !q.Empty() && n.Parent != nil && n.Parent.Kind == doc.Object && q.has(n.Key)
}

// MatchValue reports whether n's scalar value matches. Strings are searched as decoded text.
func (q Query) MatchValue(n *doc.Node) bool {
	if q.mode != contains && n.Kind != doc.String {
		return false
	}
	return !q.Empty() && !n.IsContainer() && q.has(n.Raw)
}

// Match reports whether n matches on its key or its value.
func (q Query) Match(n *doc.Node) bool { return q.MatchKey(n) || q.MatchValue(n) }

// All returns every matching node in document order.
func All(root *doc.Node, q Query) []*doc.Node {
	var out []*doc.Node
	if q.Empty() {
		return out
	}
	root.Walk(func(n *doc.Node) bool {
		if q.Match(n) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// Refines reports whether everything q matches also matches prev, so that Filter over prev's
// matches gives q's. Typing one more character is the usual case, and spares a walk.
func (q Query) Refines(prev Query) bool {
	if q.Empty() || prev.Empty() || q.Fold != prev.Fold || q.mode < prev.mode {
		return false
	}
	switch prev.mode {
	case prefix:
		return strings.HasPrefix(q.cmp, prev.cmp)
	case exact:
		return q.cmp == prev.cmp
	}
	return strings.Contains(q.cmp, prev.cmp)
}

// Filter returns the nodes of ns that match q, in the same order.
func Filter(ns []*doc.Node, q Query) []*doc.Node {
	var out []*doc.Node
	for _, n := range ns {
		if q.Match(n) {
			out = append(out, n)
		}
	}
	return out
}

// Index returns n's position in matches, or -1 when it is not one of them.
func Index(matches []*doc.Node, n *doc.Node) int {
	i := sort.Search(len(matches), func(k int) bool { return matches[k].Seq >= n.Seq })
	if i < len(matches) && matches[i] == n {
		return i
	}
	return -1
}

// Next picks the match after `from` (or before it, going back), in document order, wrapping
// at the ends as vim does. It returns the index into matches and whether it wrapped;
// -1 when there are no matches.
func Next(matches []*doc.Node, from *doc.Node, forward bool) (i int, wrapped bool) {
	if len(matches) == 0 {
		return -1, false
	}
	// first match strictly after from
	i = sort.Search(len(matches), func(k int) bool { return matches[k].Seq > from.Seq })
	if forward {
		if i == len(matches) {
			return 0, true
		}
		return i, false
	}
	// last match strictly before from
	j := sort.Search(len(matches), func(k int) bool { return matches[k].Seq >= from.Seq }) - 1
	if j < 0 {
		return len(matches) - 1, true
	}
	return j, false
}
