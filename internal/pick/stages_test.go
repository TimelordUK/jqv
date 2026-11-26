package pick

import (
	"slices"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
)

func kinds(p Pipeline) []Kind {
	var ks []Kind
	for _, s := range p {
		ks = append(ks, s.Kind)
	}
	return ks
}

// DESIGN §28.1: a program reads as stages, and the stages write back to the same program.
func TestParseStages(t *testing.T) {
	for _, c := range []struct {
		program string
		kinds   []Kind
	}{
		{"", nil},
		{".[] | {Job, Status}", []Kind{Picks}},
		{".[] | {Job} | select(.Job == \"x\")", []Kind{Picks, Filter}},
		{".[] | {Job} | select(.Job == \"x\") | select(.n > 1)", []Kind{Picks, Filter, Filter}},
		{".prizes[].laureates[]? | {id} | length", []Kind{Picks, Opaque}},
		{".[] | .Job", []Kind{Opaque, Opaque}},
		{"select(.a) | .b", []Kind{Filter, Opaque}},
		{".prizes[] as $p | $p.laureates[]? | {year: $p.year, id}", []Kind{Opaque, Opaque, Opaque}},
		{`.[] | {Job} | select(.Job == "a|b")`, []Kind{Picks, Filter}}, // a pipe in a string
		{"if . then 1 | 2 else 3 end", []Kind{Opaque}},                 // a pipe inside if…end
		{"def f: . | .; f | .a", []Kind{Opaque}},                       // and inside a def
		{".[] | {Job} | select(", []Kind{Opaque}},                      // doesn't parse: kept whole
		{".[] | {Job} | select (.a)", []Kind{Picks, Opaque}},           // not as gestures write it
		{".[] | {Job} | (select(.a) | .b)", []Kind{Picks, Opaque}},     // a pipe in brackets
	} {
		p := Parse(c.program)
		if !slices.Equal(kinds(p), c.kinds) {
			t.Errorf("Parse(%q) kinds %v, want %v", c.program, kinds(p), c.kinds)
		}
		if p.String() != c.program {
			t.Errorf("Parse(%q).String() = %q", c.program, p.String())
		}
	}
}

func TestUpto(t *testing.T) {
	p := Parse(`.[] | {Job} | select(.Job == "x") | length`)
	for n, want := range []string{"", ".[] | {Job}", `.[] | {Job} | select(.Job == "x")`, p.String()} {
		if got := p.Upto(n); got != want {
			t.Errorf("Upto(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestWithPicks(t *testing.T) {
	var p Pipeline
	f := Form{Columns: []Column{{Name: "Job", Path: []string{"Job"}}}}
	p, err := p.WithPicks(f)
	if err != nil || p.String() != ".[] | {Job}" {
		t.Fatalf("first pick: %q %v", p, err)
	}
	p, _ = p.Where(1, `.Job == "x"`, false)
	f.Columns = append(f.Columns, Column{Name: "Status", Path: []string{"Status"}})
	if p, err = p.WithPicks(f); err != nil || p.String() != `.[] | {Job, Status} | select(.Job == "x")` {
		t.Fatalf("a pick keeps the stages after: %q %v", p, err)
	}
	if _, err := p.WithPicks(Form{}); err == nil {
		t.Error("the last unpick took the picks from under a filter")
	}
	if q, err := Parse(".[] | {Job}").WithPicks(Form{}); err != nil || q != nil {
		t.Errorf("the last unpick alone: %q %v", q, err)
	}
	if _, err := Parse(".[] | .Job").WithPicks(f); err == nil {
		t.Error("picks written over a program that doesn't start with them")
	}
}

// DESIGN §28.3, the first cut: a filter goes on the end, or into the filter the cursor is on.
func TestWhere(t *testing.T) {
	p := Parse(".[] | {Job, Status}")
	p, err := p.Where(1, `.Status == "FAILURE"`, false)
	if err != nil || p.String() != `.[] | {Job, Status} | select(.Status == "FAILURE")` {
		t.Fatalf("on the end: %q %v", p, err)
	}
	p, err = p.Where(1, `.Job == "Deploy"`, false)
	if err != nil || p.String() != `.[] | {Job, Status} | select(.Status == "FAILURE" and .Job == "Deploy")` {
		t.Fatalf("into the filter: %q %v", p, err)
	}
	if _, err := p.Where(0, ".a == 1", false); err == nil {
		t.Error("a filter went into the picks")
	}
	q, _ := Parse(".[] | {a} | select(.a == 1 or .a == 2)").Where(1, ".b == 3", false)
	if want := ".[] | {a} | select((.a == 1 or .a == 2) and .b == 3)"; q.String() != want {
		t.Errorf("or kept apart: %q, want %q", q, want)
	}
	if got := Parse(q.String()); !slices.Equal(kinds(got), []Kind{Picks, Filter}) {
		t.Errorf("what Where wrote reads back as %v", kinds(got))
	}
}

// DESIGN §29: or joins a condition as and does; the conditions read left to right as added,
// with brackets only where jq's precedence (and before or) would read them otherwise.
func TestWhereOr(t *testing.T) {
	p := Parse(`.[] | {Job, Status} | select(.Status == "FAILURE")`)
	p, err := p.Where(1, `.Status == "UNKNOWN"`, true)
	if want := `.[] | {Job, Status} | select(.Status == "FAILURE" or .Status == "UNKNOWN")`; err != nil || p.String() != want {
		t.Fatalf("or: %q %v, want %q", p, err, want)
	}
	p, _ = p.Where(1, `.Job == "Deploy"`, false)
	if want := `.[] | {Job, Status} | select((.Status == "FAILURE" or .Status == "UNKNOWN") and .Job == "Deploy")`; p.String() != want {
		t.Errorf("then and: %q, want %q", p, want)
	}
	p, _ = p.Where(1, `.Job == "Build"`, true)
	if want := `.[] | {Job, Status} | select((.Status == "FAILURE" or .Status == "UNKNOWN") and .Job == "Deploy" or .Job == "Build")`; p.String() != want {
		t.Errorf("then or: %q, want %q", p, want)
	}
	// From daily use (TeamCity, Agent == X or Agent == Y): | on the end slot quietly started a
	// new filter, and the tree there only shows rows that passed. It says where or goes.
	if _, err := Parse(".[] | {a} | select(.a == 1)").Where(2, ".a == 2", true); err == nil || !strings.Contains(err.Error(), "K onto") {
		t.Errorf("or on the end: %v, want a refusal pointing at K", err)
	}
}

// From daily use: = on the end twice on the same value wrote the same filter twice. A
// condition already there is refused, as a stage of its own or as a term of the filter.
func TestWhereRefusesRepeat(t *testing.T) {
	p := Parse(`.[] | {Agent} | select(.Agent == "BLD-23-Agent4")`)
	if _, err := p.Where(2, `.Agent == "BLD-23-Agent4"`, false); err == nil {
		t.Error("the same filter again on the end")
	}
	p = Parse(`.[] | {Agent} | select(.Agent == "A" or (.Agent == "B" and .x == 1))`)
	for _, c := range []string{`.Agent == "A"`, `.Agent == "B"`, `.x==1`} {
		if _, err := p.Where(1, c, true); err == nil {
			t.Errorf("%s is in the filter already", c)
		}
	}
	if _, err := p.Where(1, `.Agent == "C"`, true); err != nil {
		t.Errorf("a new value: %v", err)
	}
	if _, err := p.Where(2, `.Agent == "A"`, false); err != nil {
		t.Errorf("a term of an or filter, on the end, narrows it: %v", err)
	}
}

// D deletes the stage under the cursor (R-014). Picks with stages after them stay, as the
// stages read their columns; on the end slot there is nothing to delete.
func TestDelete(t *testing.T) {
	p := Parse(`.[] | {Agent} | select(.Agent == "A") | select(.Agent == "A")`)
	q, err := p.Delete(2)
	if err != nil || q.String() != `.[] | {Agent} | select(.Agent == "A")` {
		t.Errorf("delete a filter: %q %v", q, err)
	}
	if _, err := p.Delete(0); err == nil {
		t.Error("deleted the picks from under their filters")
	}
	if _, err := p.Delete(3); err == nil {
		t.Error("deleted the empty slot")
	}
	if q, err := Parse(".[] | {a}").Delete(0); err != nil || q != nil {
		t.Errorf("the picks alone: %q %v", q, err)
	}
}

func TestFieldEq(t *testing.T) {
	root := load(t, "teamcity_runs.json") // read here as the stream a stage gives: one record each
	for _, c := range []struct{ path, want string }{
		{".[3].Status", `.Status == "SUCCESS"`},
		{".[3].DurationSec", ".DurationSec == 12.0"},
	} {
		n := at(t, root, c.path)
		ks, err := Field(n)
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		if got := Eq(ks, n); got != c.want {
			t.Errorf("%s: %q, want %q", c.path, got, c.want)
		}
	}
	if _, err := Field(at(t, root, ".[3]")); err == nil {
		t.Error("= on a whole record")
	}
}

// ! is = turned round (Q-071): anything but this value. A filter that would hold a value and
// its opposite together is refused, as nothing (or everything) passes it.
func TestWhereNot(t *testing.T) {
	n := &doc.Node{Kind: doc.String, Raw: "BLD-24-Agent7"}
	if got := Ne([]string{"Agent"}, n); got != `.Agent != "BLD-24-Agent7"` {
		t.Fatalf("Ne: %q", got)
	}
	p, err := Parse(".[] | {Agent, Job}").Where(1, `.Agent != "a"`, false)
	if err != nil || p.String() != `.[] | {Agent, Job} | select(.Agent != "a")` {
		t.Fatalf("on the end: %q %v", p, err)
	}
	p, err = p.Where(1, `.Agent != "b"`, false)
	if err != nil || p.String() != `.[] | {Agent, Job} | select(.Agent != "a" and .Agent != "b")` {
		t.Fatalf("another value left out: %q %v", p, err)
	}
	for _, or := range []bool{false, true} {
		if _, err := p.Where(1, `.Agent == "a"`, or); err == nil || !strings.Contains(err.Error(), `.Agent != "a"`) {
			t.Errorf("the opposite, or %v: %v", or, err)
		}
	}
}

// ~ turns the filter under the cursor round, and back (Q-073): everything but what it let
// through. A condition added after keeps the not to what it was on.
func TestNot(t *testing.T) {
	p := Parse(`.[] | {Agent, Status} | select(.Agent == "a" and .Status == "finished")`)
	p, err := p.Not(1)
	want := `.[] | {Agent, Status} | select((.Agent == "a" and .Status == "finished") | not)`
	if err != nil || p.String() != want {
		t.Fatalf("not: %q %v\n want %q", p, err, want)
	}
	if got := Parse(p.String()); !slices.Equal(kinds(got), []Kind{Picks, Filter}) {
		t.Errorf("reads back as %v", kinds(got))
	}
	q, _ := p.Where(1, `.Job == "x"`, false)
	if want := `.[] | {Agent, Status} | select(((.Agent == "a" and .Status == "finished") | not) and .Job == "x")`; q.String() != want {
		t.Errorf("and after not: %q\n want %q", q, want)
	}
	if p, _ = p.Not(1); p.String() != `.[] | {Agent, Status} | select(.Agent == "a" and .Status == "finished")` {
		t.Errorf("not again: %q", p)
	}
	if p, _ := Parse(`.[] | {a} | select(.a == 1)`).Not(1); p.String() != `.[] | {a} | select(.a == 1 | not)` {
		t.Errorf("one condition: %q", p)
	}
	for _, at := range []int{0, 2} {
		if _, err := p.Not(at); err == nil {
			t.Errorf("not at %d, not a filter", at)
		}
	}
}
