package pick

import (
	"testing"

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
	emp := load(t, "employees_5-level_10-KB_formatted.json")
	if _, err := Join(Form{}, at(t, emp, ".[0].employee.projects")); err == nil {
		t.Error("x on an array of records")
	}
	nobel := load(t, "nobel_prize.json")
	pk, _ := Lift(at(t, nobel, ".prizes[0].laureates[0].id"))
	g, _ := Toggle(Form{}, pk)
	if _, err := Join(g, at(t, org, ".[1].direct_reports")); err == nil {
		t.Error("an array outside the records picked")
	}
}
