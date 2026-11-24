package pick

import (
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/query"
)

// DESIGN §29: the parent's fields picked first, then x on direct_reports, give a row for each
// report with its manager's fields on it; x again takes the join away.
func TestJoinOrgchart(t *testing.T) {
	root := load(t, "orgchart.json")
	var f Form
	for _, p := range []string{".[1].id", ".[1].name"} {
		pk, err := Lift(at(t, root, p))
		if err != nil {
			t.Fatal(err)
		}
		if f, err = Toggle(f, pk); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Join(f, at(t, root, ".[1].direct_reports"))
	if err != nil {
		t.Fatal(err)
	}
	want := ".[] | {id, name, direct_reports: .direct_reports[]?}"
	if got := Write(f); got != want {
		t.Fatalf("Write = %q, want %q", got, want)
	}
	if back, err := Read(want); err != nil || Write(back) != want {
		t.Errorf("read back: %q %v", Write(back), err)
	}
	if !f.Joined(at(t, root, ".[4].direct_reports")) || f.Joined(at(t, root, ".[4].reports_to")) {
		t.Error("Joined marks the wrong arrays")
	}

	r := query.Run(want, root.Value(), nil, query.Options{})
	total := query.Run("[.[].direct_reports[]] | length", root.Value(), nil, query.Options{})
	if r.Err != nil || len(r.Values) != toInt(total.Values[0]) {
		t.Fatalf("%d rows, want one per report (%v): %v", len(r.Values), total.Values[0], r.Err)
	}
	if row := r.Values[0].(map[string]any); row["id"] != "E0001" || row["direct_reports"] != "E0002" {
		t.Errorf("first row %v, want the CEO and their first report", row)
	}

	// x on a value in the array is the same gesture, and again takes the join away.
	f, err = Join(f, at(t, root, ".[2].direct_reports[0]"))
	if err != nil || Write(f) != ".[] | {id, name}" {
		t.Errorf("unjoin: %q %v", Write(f), err)
	}
}

// A join reaches through arrays inside the record: each technology of each task of each
// project, with the employee's name on every row.
func TestJoinDeep(t *testing.T) {
	root := load(t, "employees_5-level_10-KB_formatted.json")
	pk, err := Lift(at(t, root, ".[0].employee.name"))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := Toggle(Form{}, pk)
	f, err = Join(f, at(t, root, ".[0].employee.projects[0].tasks[0].details.technologiesUsed"))
	if err != nil {
		t.Fatal(err)
	}
	want := ".[] | {name: .employee.name, technologiesUsed: .employee.projects[]?.tasks[]?.details.technologiesUsed[]?}"
	if got := Write(f); got != want {
		t.Fatalf("Write = %q, want %q", got, want)
	}
	if back, err := Read(want); err != nil || Write(back) != want {
		t.Errorf("read back: %q %v", Write(back), err)
	}
	r := query.Run(want, root.Value(), nil, query.Options{})
	total := query.Run("[.[].employee.projects[]?.tasks[]?.details.technologiesUsed[]?] | length", root.Value(), nil, query.Options{})
	if r.Err != nil || len(r.Values) != toInt(total.Values[0]) || len(r.Values) == 0 {
		t.Errorf("%d rows, want %v: %v", len(r.Values), total.Values[0], r.Err)
	}
}

// With nothing picked, x sets the grain; it refuses arrays of records, a second join, and an
// array outside the records.
func TestJoinRefuses(t *testing.T) {
	org := load(t, "orgchart.json")
	f, err := Join(Form{}, at(t, org, ".[1].direct_reports"))
	if err != nil || Write(f) != ".[] | {direct_reports: .direct_reports[]?}" {
		t.Fatalf("join alone: %q %v", Write(f), err)
	}
	if _, err := Join(f, at(t, org, ".[1].reports_to")); err == nil {
		t.Error("a second join, which would multiply")
	}
	if _, err := Join(Form{}, at(t, org, ".[1].name")); err == nil {
		t.Error("x on a string that isn't in an array")
	}
	if _, err := Join(Form{}, org); err == nil {
		t.Error("x on the document's own array")
	}
	nobel := load(t, "nobel_prize.json")
	pk, _ := Lift(at(t, nobel, ".prizes[0].laureates[0].id"))
	g, _ := Toggle(Form{}, pk)
	if _, err := Join(g, at(t, org, ".[1].direct_reports")); err == nil {
		t.Error("an array outside the records picked")
	}
}

// pickAll toggles the fields at ps into f, failing the test on a refusal.
func pickAll(t *testing.T, root *doc.Node, f Form, ps ...string) Form {
	t.Helper()
	for _, p := range ps {
		pk, err := Lift(at(t, root, p))
		if err != nil {
			t.Fatal(err)
		}
		if f, err = Toggle(f, pk); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	return f
}

// DESIGN §29.3 (owner 2026-11-24): x on an array of records joins it empty, a row per
// laureate with the prize's fields on each; fields picked inside a laureate go into the
// inner object, a prize's into the outer; x again takes the join and its fields away.
func TestJoinRecords(t *testing.T) {
	root := load(t, "nobel_prize.json")
	f := pickAll(t, root, Form{}, ".prizes[0].year", ".prizes[0].category")
	f, err := Join(f, at(t, root, ".prizes[0].laureates"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Write(f), ".prizes[] | {year, category} + (.laureates[]? | {})"; got != want {
		t.Fatalf("joined empty: %q, want %q", got, want)
	}
	f = pickAll(t, root, f, ".prizes[2].laureates[0].firstname", ".prizes[1].laureates[1].surname", ".prizes[0].laureates[0].id")
	want := ".prizes[] | {year, category} + (.laureates[]? | {firstname, surname, id})"
	if got := Write(f); got != want {
		t.Fatalf("Write = %q, want %q", got, want)
	}
	if back, err := Read(want); err != nil || Write(back) != want {
		t.Errorf("read back: %q %v", Write(back), err)
	}
	if st := Parse(want); len(st) != 1 || st[0].Kind != Picks {
		t.Errorf("stages: %+v", st)
	}
	if !f.Has(mustLift(t, root, ".prizes[3].laureates[0].surname")) || !f.Has(mustLift(t, root, ".prizes[3].year")) ||
		f.Has(mustLift(t, root, ".prizes[3].laureates[0].share")) {
		t.Error("Has marks the wrong fields")
	}
	if !f.Joined(at(t, root, ".prizes[4].laureates")) || !f.Joined(at(t, root, ".prizes[4].laureates[0]")) {
		t.Error("Joined should mark laureates in every prize")
	}

	r := query.Run(want, root.Value(), nil, query.Options{})
	total := query.Run("[.prizes[].laureates[]?] | length", root.Value(), nil, query.Options{})
	if r.Err != nil || len(r.Values) != toInt(total.Values[0]) {
		t.Fatalf("%d rows, want one per laureate (%v): %v", len(r.Values), total.Values[0], r.Err)
	}
	if row := r.Values[0].(map[string]any); row["year"] != "2026" || row["category"] != "chemistry" || row["firstname"] != "Henri" {
		t.Errorf("first row %v", row)
	}

	// a prize field picked after the join goes into the outer object; unpicking works on both
	f = pickAll(t, root, f, ".prizes[0].laureates[0].id", ".prizes[1].year")
	if got := Write(f); got != ".prizes[] | {category} + (.laureates[]? | {firstname, surname})" {
		t.Errorf("unpicks: %q", got)
	}
	// x on a laureate, a record in the array, is the same gesture: it takes the join away
	f, err = Join(f, at(t, root, ".prizes[1].laureates[0]"))
	if err != nil || Write(f) != ".prizes[] | {category}" {
		t.Errorf("unjoin: %q %v", Write(f), err)
	}
}

// The inner fields' names don't clash with the outer ones, since + keeps the right's: a
// laureate's id beside a prize's would hide it.
func TestJoinRecordsNames(t *testing.T) {
	root, err := doc.ParseBytes([]byte(`{"orders":[{"id":"O1","fills":[{"id":"F1","qty":5}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	f := pickAll(t, root, Form{}, ".orders[0].id")
	if f, err = Join(f, at(t, root, ".orders[0].fills")); err != nil {
		t.Fatal(err)
	}
	f = pickAll(t, root, f, ".orders[0].fills[0].id", ".orders[0].fills[0].qty")
	want := `.orders[] | {id} + (.fills[]? | {"fills.id": .id, qty})`
	if got := Write(f); got != want {
		t.Errorf("Write = %q, want %q", got, want)
	}
	if back, err := Read(want); err != nil || Write(back) != want {
		t.Errorf("read back: %q %v", Write(back), err)
	}
}

// Refused, with a reason: a second join of either kind, and an array of arrays.
func TestJoinRecordsRefuses(t *testing.T) {
	emp := load(t, "employees_5-level_10-KB_formatted.json")
	f := pickAll(t, emp, Form{}, ".[0].employee.name")
	f, err := Join(f, at(t, emp, ".[0].employee.projects"))
	if err != nil || Write(f) != ".[] | {name: .employee.name} + (.employee.projects[]? | {})" {
		t.Fatalf("join to projects: %q %v", Write(f), err)
	}
	if _, err := Join(f, at(t, emp, ".[0].employee.projects[0].tasks")); err == nil {
		t.Error("a second join, to records")
	}
	if _, err := Join(f, at(t, emp, ".[0].employee.projects[0].tasks[0].details.technologiesUsed")); err == nil {
		t.Error("a second join, to values")
	}
	m, _ := doc.ParseBytes([]byte(`[{"grid":[[1,2],[3]]}]`))
	if _, err := Join(Form{}, at(t, m, ".[0].grid")); err == nil {
		t.Error("an array of arrays")
	}
}

func mustLift(t *testing.T, root *doc.Node, p string) Pick {
	t.Helper()
	pk, err := Lift(at(t, root, p))
	if err != nil {
		t.Fatal(err)
	}
	return pk
}
