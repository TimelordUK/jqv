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
	Grain   []string
	Columns []Column
}

// Column is one lifted field: the key it gets in each row, and its keys from the record.
type Column struct {
	Name string
	Path []string
}

// Empty says there is nothing picked, and so no program.
func (f Form) Empty() bool { return len(f.Columns) == 0 }

// Write is the form as jq: the smallest program that gives its rows. An empty form is "".
func Write(f Form) string {
	if f.Empty() {
		return ""
	}
	cols := make([]string, len(f.Columns))
	for i, c := range f.Columns {
		if len(c.Path) == 1 && c.Path[0] == c.Name {
			cols[i] = key(c.Name) // {Job} or {"@timestamp"}
		} else {
			cols[i] = key(c.Name) + ": " + path(c.Path)
		}
	}
	return path(f.Grain) + "[] | {" + strings.Join(cols, ", ") + "}"
}

// Read is the form a program was written as, or an error when the program is not one: then a
// gesture says so rather than guess. Spacing doesn't matter; anything else does.
func Read(program string) (Form, error) {
	q, err := gojq.Parse(program)
	if err != nil {
		return Form{}, err
	}
	notForm := errors.New("not a program picks wrote: expected .[] | {a, b}")
	if q.Op != gojq.OpPipe || q.Left == nil || q.Right == nil || q.Left.Term == nil || q.Right.Term == nil {
		return Form{}, notForm
	}
	grain, iter, ok := keys(q.Left.Term)
	if !ok || !iter || q.Left.Op != 0 {
		return Form{}, notForm
	}
	obj := q.Right.Term
	if q.Right.Op != 0 || obj.Type != gojq.TermTypeObject || len(obj.SuffixList) > 0 {
		return Form{}, notForm
	}
	f := Form{Grain: grain}
	for _, kv := range obj.Object.KeyVals {
		var name string
		switch {
		case kv.KeyString != nil && len(kv.KeyString.Queries) == 0:
			name = kv.KeyString.Str
		case kv.Key != "" && !strings.HasPrefix(kv.Key, "$"):
			name = kv.Key
		default:
			return Form{}, notForm
		}
		c := Column{Name: name, Path: []string{name}}
		if kv.Val != nil {
			if kv.Val.Op != 0 || kv.Val.Term == nil {
				return Form{}, notForm
			}
			p, iter, ok := keys(kv.Val.Term)
			if !ok || iter || len(p) == 0 {
				return Form{}, notForm
			}
			c.Path = p
		}
		f.Columns = append(f.Columns, c)
	}
	if len(f.Columns) == 0 {
		return Form{}, notForm
	}
	// The form must write back to the same program; anything Read let through by mistake
	// fails here instead of being rewritten.
	w, err := gojq.Parse(Write(f))
	if err != nil || w.String() != q.String() {
		return Form{}, notForm
	}
	return f, nil
}

// keys reads a term made only of key steps, perhaps ending in []: `.a."b c".d[]`.
func keys(t *gojq.Term) (ks []string, iter, ok bool) {
	switch t.Type {
	case gojq.TermTypeIdentity:
	case gojq.TermTypeIndex:
		k, ok := indexKey(t.Index)
		if !ok {
			return nil, false, false
		}
		ks = append(ks, k)
	default:
		return nil, false, false
	}
	for i, s := range t.SuffixList {
		if s.Optional {
			return nil, false, false
		}
		if s.Iter {
			if i != len(t.SuffixList)-1 {
				return nil, false, false
			}
			return ks, true, true
		}
		k, ok := indexKey(s.Index)
		if !ok {
			return nil, false, false
		}
		ks = append(ks, k)
	}
	return ks, false, true
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
	Grain []string
	Path  []string
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
	var grain []string
	for a := c.Parent; a.Parent != nil; a = a.Parent {
		if a.Parent.Kind == doc.Array {
			return Pick{}, errors.New("records inside records aren't picked yet: this array is inside another")
		}
		grain = append(grain, a.Key)
	}
	slices.Reverse(field)
	slices.Reverse(grain)
	return Pick{Grain: grain, Path: field}, nil
}

// Toggle picks p into f, or unpicks it when f has it already. The first pick sets the grain;
// a pick at another grain is refused.
func Toggle(f Form, p Pick) (Form, error) {
	if f.Empty() {
		f.Grain = slices.Clone(p.Grain)
	} else if !slices.Equal(f.Grain, p.Grain) {
		return f, fmt.Errorf("these records are at %s[], not %s[]: one array of records at a time", path(p.Grain), path(f.Grain))
	}
	cols := slices.Clone(f.Columns)
	for i, c := range cols {
		if slices.Equal(c.Path, p.Path) {
			f.Columns = slices.Delete(cols, i, i+1)
			return f, nil
		}
	}
	f.Columns = append(cols, Column{Name: name(cols, p.Path), Path: slices.Clone(p.Path)})
	return f, nil
}

// Has says whether f lifts the field at p, for marking it in the tree.
func (f Form) Has(p Pick) bool {
	if !slices.Equal(f.Grain, p.Grain) {
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
