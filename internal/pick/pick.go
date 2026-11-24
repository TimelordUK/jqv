// Package pick is the jq that picks and gestures write (DESIGN §26, Q-013): one form, written
// as text and read back from it, so nothing is kept beside the program (§25 rule 1). A
// gesture reads the bar into a Form, changes it, and writes it again.
//
// Step 1 of §26 covers the grain and the projection: `.[] | {Job, Status}`. Filters, group and
// sort join the form at step 2.
package pick

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/itchyny/gojq"
)

// Form is a program that picks wrote: where the records are, and the columns lifted from each.
type Form struct {
	// Grain is the keys from the document down to the array of records: nil for a top-level
	// array (`.[]`), ["orders"] for `.orders[]`.
	Grain []string
	// Within is the arrays the records' array is inside, outermost first, each as keys from
	// the record (or document) before it: `.prizes[].laureates[]` is Within [[prizes]] and
	// Grain [laureates]. A field picked in one laureate is lifted from every laureate of every
	// prize.
	Within  [][]string
	Columns []Column
	// Sub is a join to an array of records inside each record (Q-065, DESIGN §29.3): a row
	// for each of its records, the outer columns on every one, and its own columns beside
	// them. nil when there is none.
	Sub *Sub
}

// Sub is the records a join reaches, and the columns lifted from each: Join is the arrays from
// the outer record down to them, as Column.Join has it; Columns are from the inner record.
type Sub struct {
	Join    [][]string
	Columns []Column
}

// Column is one lifted field: the key it gets in each row, and its keys from the record. A
// join (Q-065) has Join instead of Path: the arrays from the record to an array of values,
// each by keys from the element before, and the column is that array's elements, a row for
// each, the record's other columns on every one of them.
type Column struct {
	Name string
	Path []string
	Join [][]string
}

// Empty says there is nothing picked, and so no program.
func (f Form) Empty() bool { return len(f.Columns) == 0 && f.Sub == nil }

// Write is the form as jq: the smallest program that gives its rows. An empty form is "".
func Write(f Form) string {
	if f.Empty() {
		return ""
	}
	w := iterate(append(slices.Clone(f.Within), f.Grain)) + " | " + object(f.Columns)
	if f.Sub != nil {
		w += " + (" + joinPath(f.Sub.Join) + " | " + object(f.Sub.Columns) + ")"
	}
	return w
}

// object is columns as an object literal: `{Job, Duration: .Stats.Duration}`, or `{}`.
func object(cs []Column) string {
	cols := make([]string, len(cs))
	for i, c := range cs {
		cols[i] = c.jq()
	}
	return "{" + strings.Join(cols, ", ") + "}"
}

// joinPath writes a join's arrays, each []? so a record without one gives no rows rather
// than an error: `.direct_reports[]?`, `.employee.projects[]?.tasks[]?`.
func joinPath(segs [][]string) string {
	var b strings.Builder
	for i, s := range segs {
		if len(s) > 0 || i == 0 {
			b.WriteString(path(s))
		}
		b.WriteString("[]?")
	}
	return b.String()
}

// jq is the column as an object literal writes it: `Job`, `Duration: .Stats.Duration`, or a
// join, `direct_reports: .direct_reports[]?`, which gives a row per element. The ? is so a
// record without the array gives no rows rather than an error.
func (c Column) jq() string {
	switch {
	case len(c.Join) > 0:
		return key(c.Name) + ": " + joinPath(c.Join)
	case len(c.Path) == 1 && c.Path[0] == c.Name:
		return key(c.Name) // {Job} or {"@timestamp"}
	}
	return key(c.Name) + ": " + path(c.Path)
}

// iterate writes arrays inside arrays as one jq path: `.prizes[].laureates[]?`, `.[][]?`.
func iterate(segs [][]string) string {
	var b strings.Builder
	for i, s := range segs {
		if len(s) > 0 || i == 0 {
			b.WriteString(path(s))
		}
		b.WriteString("[]")
		if i > 0 {
			b.WriteString("?") // a prize with no laureates has none to give, not an error
		}
	}
	return b.String()
}

// Read is the form a program was written as, or an error when the program is not one: then a
// gesture says so rather than guess. Spacing doesn't matter; anything else does.
func Read(program string) (Form, error) {
	q, err := gojq.Parse(program)
	if err != nil {
		return Form{}, err
	}
	notForm := errors.New("not a program picks wrote: expected .[] | {a, b}")
	if q.Op != gojq.OpPipe || q.Left == nil || q.Right == nil || q.Left.Term == nil {
		return Form{}, notForm
	}
	segs, tail, ok := keys(q.Left.Term)
	if !ok || len(segs) == 0 || len(tail) > 0 || q.Left.Op != 0 {
		return Form{}, notForm
	}
	f := Form{Grain: segs[len(segs)-1]}
	if len(segs) > 1 {
		f.Within = segs[:len(segs)-1]
	}
	right := q.Right
	if right.Op == gojq.OpAdd { // {outer} + (.join[]? | {inner}), DESIGN §29.3
		sub, ok := readSub(right.Right)
		if !ok {
			return Form{}, notForm
		}
		f.Sub, right = sub, right.Left
	}
	if right.Op != 0 || right.Term == nil {
		return Form{}, notForm
	}
	cols, ok := columns(right.Term)
	if !ok || len(cols) == 0 && f.Sub == nil {
		return Form{}, notForm
	}
	f.Columns = cols
	// The form must write back to the same program; anything Read let through by mistake
	// fails here instead of being rewritten.
	w, err := gojq.Parse(Write(f))
	if err != nil || w.String() != q.String() {
		return Form{}, notForm
	}
	return f, nil
}

// readSub reads a record join's `(.laureates[]? | {firstname})`.
func readSub(q *gojq.Query) (*Sub, bool) {
	if q == nil || q.Op != 0 || q.Term == nil || q.Term.Type != gojq.TermTypeQuery || len(q.Term.SuffixList) > 0 {
		return nil, false
	}
	in := q.Term.Query
	if in.Op != gojq.OpPipe || in.Left == nil || in.Left.Term == nil || in.Left.Op != 0 ||
		in.Right == nil || in.Right.Op != 0 || in.Right.Term == nil {
		return nil, false
	}
	segs, tail, ok := keys(in.Left.Term)
	if !ok || len(segs) == 0 || len(tail) > 0 {
		return nil, false
	}
	cols, ok := columns(in.Right.Term)
	if !ok || slices.ContainsFunc(cols, func(c Column) bool { return len(c.Join) > 0 }) {
		return nil, false
	}
	return &Sub{Join: segs, Columns: cols}, true
}

// columns reads an object literal of picks: `{Job, Duration: .Stats.Duration}`, joins too.
func columns(obj *gojq.Term) ([]Column, bool) {
	if obj.Type != gojq.TermTypeObject || len(obj.SuffixList) > 0 {
		return nil, false
	}
	cols := []Column{}
	for _, kv := range obj.Object.KeyVals {
		var name string
		switch {
		case kv.KeyString != nil && len(kv.KeyString.Queries) == 0:
			name = kv.KeyString.Str
		case kv.Key != "" && !strings.HasPrefix(kv.Key, "$"):
			name = kv.Key
		default:
			return nil, false
		}
		c := Column{Name: name, Path: []string{name}}
		if kv.Val != nil {
			if kv.Val.Op != 0 || kv.Val.Term == nil {
				return nil, false
			}
			segs, p, ok := keys(kv.Val.Term)
			switch {
			case ok && len(segs) == 0 && len(p) > 0:
				c.Path = p
			case ok && len(segs) > 0 && len(p) == 0: // a join, ending at the array
				c.Path, c.Join = nil, segs
			default:
				return nil, false
			}
		}
		cols = append(cols, c)
	}
	return cols, true
}

// keys reads a term made only of key steps and []: `.a."b c"[].d` is segments [[a, "b c"]]
// (each ending in []) and tail [d].
func keys(t *gojq.Term) (segs [][]string, tail []string, ok bool) {
	tail = []string{}
	switch t.Type {
	case gojq.TermTypeIdentity:
	case gojq.TermTypeIndex:
		k, ok := indexKey(t.Index)
		if !ok {
			return nil, nil, false
		}
		tail = append(tail, k)
	default:
		return nil, nil, false
	}
	for i, s := range t.SuffixList {
		if s.Iter {
			// Write puts ? on every [] but the first; anything else fails Read's check.
			if len(tail) == 0 {
				tail = nil
			}
			segs, tail = append(segs, tail), []string{}
			continue
		}
		if s.Optional {
			if i == 0 || !t.SuffixList[i-1].Iter {
				return nil, nil, false
			}
			continue
		}
		k, ok := indexKey(s.Index)
		if !ok {
			return nil, nil, false
		}
		tail = append(tail, k)
	}
	return segs, tail, true
}

func indexKey(x *gojq.Index) (string, bool) {
	switch {
	case x == nil || x.IsSlice || x.Start != nil || x.End != nil:
		return "", false
	case x.Str != nil && len(x.Str.Queries) == 0:
		return x.Str.Str, true
	case x.Name != "":
		return x.Name, true
	}
	return "", false
}

// Pick is a field the cursor is on, generalised to every record (Q-006): on `.[3].Stats.Ms`
// the grain is the top-level array and the field Stats.Ms.
type Pick struct {
	Within [][]string
	Grain  []string
	Path   []string
}

// Lift generalises the cursor's node to a pick, or says why it can't be one.
func Lift(n *doc.Node) (Pick, error) {
	if n.IsContainer() {
		return Pick{}, errors.New("pick a field with a value, not a whole object or array")
	}
	var field []string
	c := n
	for c.Parent != nil && c.Parent.Kind == doc.Object {
		field = append(field, c.Key)
		c = c.Parent
	}
	// c is now the record, if there is one: an object that is an element of an array.
	if c.Parent == nil {
		return Pick{}, errors.New("pick a field inside an array of records")
	}
	if len(field) == 0 || c.Kind != doc.Object {
		return Pick{}, errors.New("pick a field inside an array of records, not an array's own value")
	}
	// Climb from the records' array to the document, an array at a time: each is reached
	// by keys from the record (or document) above it.
	var segs [][]string
	for a := c.Parent; ; {
		var seg []string
		n := a
		for n.Parent != nil && n.Parent.Kind == doc.Object {
			seg = append(seg, n.Key)
			n = n.Parent
		}
		slices.Reverse(seg)
		segs = append(segs, seg)
		if n.Parent == nil {
			break
		}
		a = n.Parent // n is an element of an outer array
	}
	slices.Reverse(segs)
	slices.Reverse(field)
	p := Pick{Grain: segs[len(segs)-1], Path: field}
	if len(segs) > 1 {
		p.Within = segs[:len(segs)-1]
	}
	return p, nil
}

func (f Form) sameGrain(p Pick) bool {
	return slices.Equal(f.Grain, p.Grain) && slices.EqualFunc(f.Within, p.Within, slices.Equal)
}

// Toggle picks p into f, or unpicks it when f has it already. The first pick sets the grain;
// a pick at another grain is refused.
func Toggle(f Form, p Pick) (Form, error) {
	if f.inSub(p) { // a field of the joined records (DESIGN §29.3)
		sub := *f.Sub
		sub.Columns = slices.Clone(sub.Columns)
		for i, c := range sub.Columns {
			if slices.Equal(c.Path, p.Path) {
				sub.Columns = slices.Delete(sub.Columns, i, i+1)
				f.Sub = &sub
				return f, nil
			}
		}
		// named apart from the outer columns: + keeps the right's, so a clash would hide one
		last := sub.Join[len(sub.Join)-1]
		n := name(append(slices.Clone(f.Columns), sub.Columns...), append(slices.Clone(last), p.Path...))
		if len(last) > 0 && !strings.Contains(n, ".") && slices.ContainsFunc(f.Columns, func(c Column) bool { return c.Name == n }) {
			n = strings.Join(append(slices.Clone(last), p.Path...), ".")
		}
		sub.Columns = append(sub.Columns, Column{Name: n, Path: slices.Clone(p.Path)})
		f.Sub = &sub
		return f, nil
	}
	if f.Empty() {
		f.Grain, f.Within = slices.Clone(p.Grain), slices.Clone(p.Within)
	} else if !f.sameGrain(p) {
		return f, fmt.Errorf("these records are at %s, not %s: one array of records at a time",
			iterate(append(slices.Clone(p.Within), p.Grain)), iterate(append(slices.Clone(f.Within), f.Grain)))
	}
	cols := slices.Clone(f.Columns)
	for i, c := range cols {
		if slices.Equal(c.Path, p.Path) {
			f.Columns = slices.Delete(cols, i, i+1)
			return f, nil
		}
	}
	taken := cols
	if f.Sub != nil {
		taken = append(slices.Clone(cols), f.Sub.Columns...)
	}
	f.Columns = append(cols, Column{Name: name(taken, p.Path), Path: slices.Clone(p.Path)})
	return f, nil
}

// inSub says whether p is a field of the records f's record join reaches.
func (f Form) inSub(p Pick) bool {
	if f.Sub == nil {
		return false
	}
	chain := append(append(slices.Clone(f.Within), f.Grain), f.Sub.Join...)
	return slices.EqualFunc(append(slices.Clone(p.Within), p.Grain), chain, slices.Equal)
}

// Has says whether f lifts the field at p, for marking it in the tree.
func (f Form) Has(p Pick) bool {
	if f.inSub(p) {
		return slices.ContainsFunc(f.Sub.Columns, func(c Column) bool { return slices.Equal(c.Path, p.Path) })
	}
	if !f.sameGrain(p) {
		return false
	}
	return slices.ContainsFunc(f.Columns, func(c Column) bool { return slices.Equal(c.Path, p.Path) })
}

// name is a new column's key: the field's own name, or with its parents' in front when that
// is taken (`id`, then `instrument.id`), as GRID §4 has it.
func name(cols []Column, p []string) string {
	taken := func(n string) bool {
		return slices.ContainsFunc(cols, func(c Column) bool { return c.Name == n })
	}
	for i := len(p) - 1; i >= 0; i-- {
		if n := strings.Join(p[i:], "."); !taken(n) {
			return n
		}
	}
	base := strings.Join(p, ".")
	for k := 2; ; k++ {
		if n := fmt.Sprintf("%s_%d", base, k); !taken(n) {
			return n
		}
	}
}

// path writes key steps as jq: `.a."b c"`, or "." for none.
func path(ks []string) string {
	if len(ks) == 0 {
		return "."
	}
	var b strings.Builder
	for _, k := range ks {
		b.WriteString("." + key(k))
	}
	return b.String()
}

// key is an object key as jq writes it: bare when it can be (GRID trap 18).
func key(k string) string {
	if ident(k) {
		return k
	}
	q, _ := json.Marshal(k)
	return string(q)
}

// keywords are quoted too: `{if}` and `.end` don't parse in every jq.
var keywords = map[string]bool{
	"and": true, "or": true, "if": true, "then": true, "elif": true, "else": true, "end": true,
	"as": true, "def": true, "reduce": true, "foreach": true, "try": true, "catch": true,
	"label": true, "import": true, "include": true, "__loc__": true,
}

func ident(s string) bool {
	if s == "" || keywords[s] {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// Pretty lays the form out for a pane width cells wide: on one line when it fits, else the
// grain on its own line and the columns packed under it, a few to a line (DESIGN §26, the
// read-only bar). The lines join with spaces to a program that reads back as f.
func Pretty(f Form, width int) []string {
	one := Write(f)
	if f.Empty() || len(one) <= width {
		return []string{one}
	}
	lines := []string{iterate(append(slices.Clone(f.Within), f.Grain)) + " | {"}
	lines = pack(lines, f.Columns, width)
	if f.Sub == nil {
		return append(lines, "}")
	}
	lines = append(lines, "} + ("+joinPath(f.Sub.Join)+" | {")
	return append(pack(lines, f.Sub.Columns, width), "})")
}

// pack adds columns to lines, a few to a line, indented, with commas between.
func pack(lines []string, cs []Column, width int) []string {
	line := ""
	for i, c := range cs {
		item := c.jq()
		if i < len(cs)-1 {
			item += ","
		}
		switch {
		case line == "":
			line = "    " + item
		case len(line)+1+len(item) <= width:
			line += " " + item
		default:
			lines = append(lines, line)
			line = "    " + item
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
