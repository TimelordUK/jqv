package pick

import (
	"errors"
	"fmt"
	"slices"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Join adds a cross join to the array at n, or to the array n is an element of (Q-065, DESIGN
// §29). An array of values is a column of its elements, a row for each, with the record's
// other columns on every row. An array of records joins empty (§29.3): a row for each of its
// records, the outer columns on each, and fields picked inside one go beside them. Again on
// the same array takes it away, with any fields picked in it. With nothing picked it sets
// the grain to the outermost array of records, as `.[]` in orgchart.json. One join at a time:
// two would multiply.
func Join(f Form, n *doc.Node) (Form, error) {
	join, grain, records, err := joinOf(f, n)
	if err != nil {
		return f, err
	}
	if f.Sub != nil && slices.EqualFunc(f.Sub.Join, join, slices.Equal) {
		f.Sub = nil
		return f, nil
	}
	if records {
		if f.Sub != nil || slices.ContainsFunc(f.Columns, func(c Column) bool { return len(c.Join) > 0 }) {
			return f, errors.New("one join at a time: x on the joined array takes it away")
		}
		if f.Empty() {
			f.Within, f.Grain = grain[:len(grain)-1], grain[len(grain)-1]
		}
		f.Sub = &Sub{Join: join}
		return f, nil
	}
	cols := slices.Clone(f.Columns)
	for i, c := range cols {
		if slices.EqualFunc(c.Join, join, slices.Equal) {
			f.Columns = slices.Delete(cols, i, i+1)
			return f, nil
		}
	}
	if f.Sub != nil || slices.ContainsFunc(cols, func(c Column) bool { return len(c.Join) > 0 }) {
		return f, errors.New("one join at a time: x on the joined array takes it away")
	}
	if f.Empty() {
		f.Within, f.Grain = grain[:len(grain)-1], grain[len(grain)-1]
	}
	f.Columns = append(cols, Column{Name: name(cols, join[len(join)-1]), Join: join})
	return f, nil
}

// Joined says whether f joins the array at n (or, for records, the array n is in), for
// marking it in the tree.
func (f Form) Joined(n *doc.Node) bool {
	if n.Kind == doc.Object && n.Parent != nil && n.Parent.Kind == doc.Array {
		n = n.Parent
	}
	if f.Empty() || n.Kind != doc.Array {
		return false
	}
	join, _, _, err := joinOf(f, n)
	if err != nil {
		return false
	}
	if f.Sub != nil && slices.EqualFunc(f.Sub.Join, join, slices.Equal) {
		return true
	}
	return slices.ContainsFunc(f.Columns, func(c Column) bool {
		return slices.EqualFunc(c.Join, join, slices.Equal)
	})
}

// joinOf is the join to the array at n (or holding n) from f's records, the grain those
// records are at (f's, or with nothing picked, the outermost array above the array), and
// whether the array holds records rather than values.
func joinOf(f Form, n *doc.Node) (join, grain [][]string, records bool, err error) {
	a := n
	if a.Kind != doc.Array && a.Parent != nil && a.Parent.Kind == doc.Array {
		a = a.Parent
	}
	if a.Kind != doc.Array {
		return nil, nil, false, errors.New("x joins an array: on one, or on an element of one")
	}
	values := 0
	for _, c := range a.Children {
		switch {
		case c.Kind == doc.Array:
			return nil, nil, false, errors.New("x joins an array of values or of records, not of arrays")
		case c.Kind == doc.Object:
			records = true
		case c.Kind != doc.Null:
			values++
		}
	}
	if records && values > 0 {
		return nil, nil, false, errors.New("this array holds records and values both: x joins one or the other")
	}
	chain := arrays(a)
	grain = chain[:1]
	if !f.Empty() {
		grain = append(slices.Clone(f.Within), f.Grain)
	}
	if len(chain) <= len(grain) || !slices.EqualFunc(chain[:len(grain)], grain, slices.Equal) {
		if len(chain) == 1 {
			return nil, nil, false, errors.New("x joins an array inside a record, to the record's other fields")
		}
		return nil, nil, false, fmt.Errorf("this array isn't inside the records at %s", iterate(grain))
	}
	join = slices.Clone(chain[len(grain):])
	if last := join[len(join)-1]; len(last) == 0 {
		return nil, nil, false, errors.New("an array straight inside an array has no name to join as")
	}
	return join, grain, records, nil
}

// arrays is the arrays from the document down to a, outermost first, each as keys from the
// element (or document) before it; the last is a itself. orgchart.json's
// `.[3].direct_reports` is [[] [direct_reports]]: `.[]`, then `.direct_reports[]`.
func arrays(a *doc.Node) [][]string {
	var segs [][]string
	for n := a; ; {
		var seg []string
		for n.Parent != nil && n.Parent.Kind == doc.Object {
			seg = append(seg, n.Key)
			n = n.Parent
		}
		slices.Reverse(seg)
		segs = append(segs, seg)
		if n.Parent == nil {
			break
		}
		n = n.Parent // n is an element of an outer array
	}
	slices.Reverse(segs)
	return segs
}
