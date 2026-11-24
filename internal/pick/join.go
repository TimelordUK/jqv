package pick

import (
	"errors"
	"fmt"
	"slices"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Join adds a cross join to the array at n, or to the array n is a value in (Q-065, DESIGN
// §29): a column of the array's elements, a row for each, with the record's other columns
// on every row. Again on the same array takes it away. With nothing picked it sets the grain
// to the outermost array of records, as `.[]` in orgchart.json. For now the array must hold
// values, not records, and there is one join at a time: two would multiply.
func Join(f Form, n *doc.Node) (Form, error) {
	join, grain, err := joinOf(f, n)
	if err != nil {
		return f, err
	}
	cols := slices.Clone(f.Columns)
	for i, c := range cols {
		if slices.EqualFunc(c.Join, join, slices.Equal) {
			f.Columns = slices.Delete(cols, i, i+1)
			return f, nil
		}
	}
	if slices.ContainsFunc(cols, func(c Column) bool { return len(c.Join) > 0 }) {
		return f, errors.New("one join at a time: x on the joined array takes it away")
	}
	if f.Empty() {
		f.Within, f.Grain = grain[:len(grain)-1], grain[len(grain)-1]
	}
	f.Columns = append(cols, Column{Name: name(cols, join[len(join)-1]), Join: join})
	return f, nil
}

// Joined says whether f joins the array at n, for marking it in the tree.
func (f Form) Joined(n *doc.Node) bool {
	if f.Empty() || n.Kind != doc.Array {
		return false
	}
	join, _, err := joinOf(f, n)
	return err == nil && slices.ContainsFunc(f.Columns, func(c Column) bool {
		return slices.EqualFunc(c.Join, join, slices.Equal)
	})
}

// joinOf is the join to the array at n (or holding n) from f's records, and the grain those
// records are at: f's, or with nothing picked, the outermost array above the array.
func joinOf(f Form, n *doc.Node) (join, grain [][]string, err error) {
	a := n
	if !a.IsContainer() && a.Parent != nil && a.Parent.Kind == doc.Array {
		a = a.Parent
	}
	if a.Kind != doc.Array {
		return nil, nil, errors.New("x joins an array: on one, or on a value in one")
	}
	for _, c := range a.Children {
		if c.IsContainer() {
			return nil, nil, errors.New("a join to an array of records comes next: x on an array of values for now")
		}
	}
	chain := arrays(a)
	grain = chain[:1]
	if !f.Empty() {
		grain = append(slices.Clone(f.Within), f.Grain)
	}
	if len(chain) <= len(grain) || !slices.EqualFunc(chain[:len(grain)], grain, slices.Equal) {
		if len(chain) == 1 {
			return nil, nil, errors.New("x joins an array inside a record, to the record's other fields")
		}
		return nil, nil, fmt.Errorf("this array isn't inside the records at %s", iterate(grain))
	}
	join = slices.Clone(chain[len(grain):])
	if last := join[len(join)-1]; len(last) == 0 {
		return nil, nil, errors.New("an array straight inside an array has no name to join as")
	}
	return join, grain, nil
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
