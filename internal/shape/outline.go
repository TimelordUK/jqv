package shape

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Options shape an outline.
type Options struct {
	Depth int // levels below the root to show; 0 shows all
}

// Enum is the most distinct values a scalar place can have for them to be listed, rather than
// counted: few enough to be a set of codes (`"Y" "N"`), the values worth a select.
const Enum = 5

// Outline is the shape as indented lines, one place a line, each labelled as a jq path step
// (`.orders`, `[]`) and described: its types, how many, array lengths, and for scalars the
// values when few, or how many distinct and an example. An array place with only arrays in
// it shares its line with its elements (`.fills[]`), and an object with only one key with
// that key (`.fix.messages`), so regular documents read short.
//
// A place that is sometimes an array and sometimes an object gets a note at the end, with
// the jq that walks both: XML converted to JSON gives a lone child as an object, so `.field[]`
// on it walks the child's values instead of the one child.
func Outline(s *Shape, o Options) string {
	ol := &outline{o: o}
	ol.walk(s, nil, "", 0)
	rows := ol.rows
	w := 0
	for _, r := range rows {
		w = max(w, len([]rune(r[0])))
	}
	w = min(w, 48)
	var b strings.Builder
	for _, r := range rows {
		pad := w - len([]rune(r[0]))
		b.WriteString(r[0])
		if r[1] != "" {
			b.WriteString(strings.Repeat(" ", max(pad, 0)+2) + r[1])
		}
		b.WriteByte('\n')
	}
	for _, p := range ol.mixed {
		fmt.Fprintf(&b, "⚠ %s is an array or a single object; to walk both: %s | if type == \"array\" then .[] else . end\n", p, p)
	}
	return b.String()
}

type outline struct {
	o     Options
	rows  [][2]string
	mixed []string // generalised paths that are both array and object
}

// walk adds s's row and its children's. parent is the place s is a key or the elements of,
// for how often s is present; path is the generalised path to parent (`.orders[]`), and depth
// 0 is the root.
func (ol *outline) walk(s, parent *Shape, path string, depth int) {
	label := ""
	if s != nil && parent == nil {
		label = "."
	} else {
		label = step(s)
	}
	lenOf := (*Shape)(nil) // the array place folded into this line, for its lengths
	for {
		if foldedArray(s) {
			lenOf, parent, s = s, s, s.elem
			label = join(label, "[]")
			continue
		}
		if k, ok := s.Only(); ok && k == doc.Object && len(s.Children) == 1 && s.elem == nil && lenOf == nil {
			if c := s.Children[0]; c.Count == s.Count {
				parent, s = s, c
				label = join(label, step(c))
				continue
			}
		}
		break
	}
	path = join(path, label)
	text := describe(s, presentIn(parent))
	if lenOf != nil {
		text = strings.TrimSpace(lengths(lenOf) + "  " + text)
	}
	if s.Kinds[doc.Array] > 0 && s.Kinds[doc.Object] > 0 {
		ol.mixed = append(ol.mixed, path)
	}
	ol.rows = append(ol.rows, [2]string{strings.Repeat("  ", depth) + label, text})
	if ol.o.Depth > 0 && depth >= ol.o.Depth {
		if n := below(s); n > 0 {
			ol.rows = append(ol.rows, [2]string{strings.Repeat("  ", depth+1) + "…", fmt.Sprintf("%d place%s below", n, plural(n))})
		}
		return
	}
	for _, c := range s.Children {
		ol.walk(c, s, path, depth+1)
	}
}

// join appends a path step: `.` then `.fix` is `.fix`, not `..fix`.
func join(path, step string) string {
	if path == "" {
		return step
	}
	if path == "." && strings.HasPrefix(step, ".") {
		return step
	}
	if path == "." {
		return "." + step
	}
	return path + step
}

// presentIn is how many values could have had s: the objects in parent for a key, the array
// elements for [].
func presentIn(parent *Shape) int {
	if parent == nil {
		return 0
	}
	return parent.Kinds[doc.Object]
}

func below(s *Shape) int {
	n := 0
	for _, c := range s.Children {
		n += 1 + below(c)
	}
	return n
}

func step(s *Shape) string {
	if s.Elem {
		return "[]"
	}
	if isIdent(s.Key) {
		return "." + s.Key
	}
	q, _ := json.Marshal(s.Key)
	return "." + string(q)
}

// describe is s's types, then what else is worth knowing. of is how many objects s could have
// been a key of: when s turned up in fewer, the line says so (`missing in 1`), since that key
// needs care in a select.
func describe(s *Shape, of int) string {
	var parts []string
	var kinds []string
	for k, c := range s.Kinds {
		if c > 0 {
			kinds = append(kinds, fmt.Sprintf("%s ×%d", doc.Kind(k), c))
		}
	}
	if _, ok := s.Only(); ok {
		kinds[0] = strings.Fields(kinds[0])[0]
		if s.Count > 1 {
			kinds[0] += " ×" + strconv.Itoa(s.Count)
		}
	}
	parts = append(parts, strings.Join(kinds, " | "))
	if !s.Elem && of > s.Count {
		parts = append(parts, fmt.Sprintf("missing in %d", of-s.Count))
	}
	if s.Kinds[doc.Array] > 0 && !foldedArray(s) {
		parts = append(parts, lengths(s))
	}
	if v := scalars(s); v != "" {
		parts = append(parts, v)
	}
	if s.Mixed() {
		parts = append(parts, "⚠ mixed")
	}
	return strings.Join(parts, "  ")
}

// foldedArray reports whether s shares its line with its elements (walk), which then shows
// the lengths.
func foldedArray(s *Shape) bool {
	k, ok := s.Only()
	return ok && k == doc.Array && s.elem != nil && len(s.Children) == 1
}

func lengths(s *Shape) string {
	if s.Kinds[doc.Array] == 0 {
		return ""
	}
	if s.MinLen == s.MaxLen {
		return "len " + strconv.Itoa(s.MinLen)
	}
	return fmt.Sprintf("len %d..%d", s.MinLen, s.MaxLen)
}

// scalars is what the scalars here were: the values when few, else how many distinct, the
// range of the numbers, and an example.
func scalars(s *Shape) string {
	n := s.Kinds[doc.String] + s.Kinds[doc.Number] + s.Kinds[doc.Bool] + s.Kinds[doc.Null]
	if n == 0 {
		return ""
	}
	d, more := s.Distinct()
	if s.Count == 1 && d == 1 {
		return clip(s.order[0], 40)
	}
	if !more && d <= Enum && (d < n || n <= 3) {
		var vs []string
		for _, v := range s.Top(Enum) {
			t := clip(v.Text, 24)
			if v.N > 1 {
				t += fmt.Sprintf(" ×%d", v.N)
			}
			vs = append(vs, t)
		}
		return strings.Join(vs, " ")
	}
	var parts []string
	switch {
	case more:
		parts = append(parts, fmt.Sprintf("%d+ distinct", MaxDistinct))
	case d == n:
		parts = append(parts, "all distinct")
	default:
		parts = append(parts, fmt.Sprintf("%d distinct", d))
	}
	if s.Kinds[doc.Number] > 1 {
		parts = append(parts, num(s.MinNum)+".."+num(s.MaxNum))
	}
	if e, ok := s.example(); ok {
		parts = append(parts, "e.g. "+clip(e, 32))
	}
	return strings.Join(parts, "  ")
}

func num(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// clip shortens text to n runes, ending in … when cut.
func clip(t string, n int) string {
	r := []rune(t)
	if len(r) <= n {
		return t
	}
	return string(r[:n-1]) + "…"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// example is the first value seen here that isn't null.
func (s *Shape) example() (string, bool) {
	for _, v := range s.order {
		if v != "null" {
			return v, true
		}
	}
	return "", false
}
