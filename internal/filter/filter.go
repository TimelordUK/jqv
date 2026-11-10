// Package filter is the filter stack (DESIGN §10.1): layers of jq conditions that narrow an
// array of records without reshaping them, like a structural grep.
//
// A layer is plain jq, run on one record. The record stays when the layer gives a true value
// (anything but false or null) at least once, so a layer that answers several times, such as
// `.parties[].role == "seller"`, keeps a record once. A layer that fails on a record (indexing
// a string, iterating null) drops it, and the failures are counted so they can be reported:
// in a structural grep a record a condition can't apply to is not a match. Lift writes the same
// meaning as one ordinary jq program, and the corpus (testdata/filter) holds the two to agreeing,
// under gojq and real jq.
package filter

import (
	"fmt"
	"os"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/itchyny/gojq"
)

// Layer is one compiled condition.
type Layer struct {
	Expr string
	code *gojq.Code
}

// Compile parses a layer. A top-level single = is refused: in jq it assigns, and since the
// result is a record, which is true, every record would pass.
func Compile(expr string) (Layer, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return Layer{}, err
	}
	if a := assignment(q); a != nil {
		return Layer{}, fmt.Errorf("a single = assigns in jq; == compares: %s == %s", a.Left, a.Right)
	}
	code, err := gojq.Compile(q, gojq.WithEnvironLoader(os.Environ)) // $ENV, as in jq
	if err != nil {
		return Layer{}, err
	}
	return Layer{Expr: expr, code: code}, nil
}

// assignment finds an = among the operators joining the query's top level (pipes, and, or).
func assignment(q *gojq.Query) *gojq.Query {
	if q == nil {
		return nil
	}
	if q.Op == gojq.OpAssign {
		return q
	}
	if a := assignment(q.Left); a != nil {
		return a
	}
	return assignment(q.Right)
}

// pass runs the layer on one record's value: does it say true at least once?
func (l Layer) pass(v any) (bool, error) {
	it := l.code.Run(v)
	for {
		o, ok := it.Next()
		if !ok {
			return false, nil
		}
		if err, ok := o.(error); ok {
			return false, err
		}
		if o != nil && o != false {
			return true, nil
		}
	}
}

// Result is what a stack leaves of an array.
type Result struct {
	Kept   []*doc.Node // the records left, in document order
	Errors int         // records dropped because a layer failed on them
	Err    error       // the first of those failures, for the status line
}

// Apply runs the stack over the elements of records, top layer first. An empty stack keeps
// everything.
func Apply(records *doc.Node, layers []Layer) Result {
	var r Result
next:
	for _, c := range records.Children {
		v := c.Value()
		for _, l := range layers {
			ok, err := l.pass(v)
			if err != nil {
				r.Errors++
				if r.Err == nil {
					r.Err = fmt.Errorf("%s: %s: %w", c.Path(), l.Expr, err)
				}
				continue next
			}
			if !ok {
				continue next
			}
		}
		r.Kept = append(r.Kept, c)
	}
	return r
}

// Lift writes the stack over the array at path as one jq program giving the whole document
// with only the kept records: `.trades |= map(select(any((.qty > 50)?; .)))`. any(...; .)
// keeps a record once however often a layer says true, and ? drops it where a layer fails.
func Lift(path string, layers []Layer) string {
	if len(layers) == 0 {
		return "."
	}
	sel := make([]string, len(layers))
	for i, l := range layers {
		sel[i] = "select(any((" + l.Expr + ")?; .))"
	}
	return path + " |= map(" + strings.Join(sel, " | ") + ")"
}
