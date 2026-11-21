package scope

import (
	"fmt"
	"strings"
	"testing"
)

// Checks (Q-046, DESIGN §22) on the employees document: each as the text it covers, its
// level and its suggestions.
func TestCheck(t *testing.T) {
	s, _ := shapes(t, "employees_5-level_10-KB_formatted.json")
	for _, c := range []struct {
		line string
		want string // `text level [suggestions]`, ; between checks
	}{
		// the owner's line: six silent nulls
		{`.[].employee | {"eid": .id, "ename": .name, "epos": .position, "pid": .projects[0].id}`, `.id warning [projectId]`},
		{`.[].employee | .nmae`, `.nmae warning [name]`},         // a swap is one edit
		{`.[].employee as $e | $e.nmae`, `.nmae warning [name]`}, // from a variable
		{`.[].employee | .na`, `.na note [name]`},                // being typed
		{`.[].employee.junk.more`, `.junk warning []`},           // the first wrong step only
		{`.[] | .x`, `.x warning []`},
		// jq's errors
		{`.[].employee | .projects.x`, `.x error [.projects[].x .projects[0].x]`},
		{`.x`, `.x error [.[].x .[0].x]`},
		{`.[].employee.name.x`, `.x error []`},
		// indexes
		{`.[].employee.projects[9]`, `[9] warning []`},
		{`.[].employee.projects[-2]`, `[-2] warning []`},
		{`.[].employee.projects[].tasks[].details.technologiesUsed[3]`, `[3] note []`},
		// a function that takes the whole array, after [] has sent the elements in one by one
		// (owner, 2026-11-14: the mistake made most often)
		{`.[].employee | {"t": .projects[0].tasks[] | map(.taskId)}`, `map warning [.projects[0].tasks | map(.taskId) .projects[0].tasks[] | .taskId]`},
		{`.[].employee.projects[].tasks[] | sort_by(.status)`, `sort_by warning [.[].employee.projects[].tasks | sort_by(.status)]`},
		{`.[].employee.name[] | map(.x)`, ``}, // the [] fails first: jq's own error
		{`.[].employee.projects[0].tasks | map(.taskId)`, ``},
		{`.[].employee | map(.x)`, `map note []`}, // map on an object goes over its values
		{`.[].employee | sort_by(.x)`, `sort_by error []`},
		{`.[].employee.name | map(.)`, `map error []`},
		// optional keys
		{`.[].employee.projects[].tasks[].details.completionDate`, `.completionDate note []`},
		// nothing to say
		{`.[].employee.projects[-1].projectId`, ``},
		{`.[].employee.junk?`, ``},
		{`.[].employee.projects[9]?`, ``},
		{`def f: .zz; .[].employee | map(.qq)`, `map note []`}, // not the def's body; map on an object, a note
		{`.[].employee | {eid: .id, name}`, ``},
	} {
		var got []string
		rs := []rune(c.line)
		for _, d := range Check(c.line, s) {
			got = append(got, fmt.Sprintf("%s %s %v", string(rs[d.From:d.To]), d.Level, d.Suggest))
		}
		if g := strings.Join(got, "; "); g != c.want {
			t.Errorf("%s\n  got  %s\n  want %s", c.line, g, c.want)
		}
	}
}

// The messages say what jq does and what to write.
func TestCheckMessages(t *testing.T) {
	s, _ := shapes(t, "employees_5-level_10-KB_formatted.json")
	for _, c := range []struct{ line, msg string }{
		{`.[].employee | .projects[0].id`, `.id: no such key in the 6 objects here, so jq gives null; did you mean .projectId?`},
		{`.[].employee | .projects.x`, `.x: .projects is an array here; a key is asked of its elements: .projects[].x or .projects[0].x`},
		{`.[].employee.projects[9]`, `[9]: .[].employee.projects has 1 element in all 6 arrays here, so [9] is always null`},
		{`.[].employee.projects[].tasks[].details.technologiesUsed[3]`, `[3]: .[].employee.projects[].tasks[].details.technologiesUsed has 2..4 elements; [3] is null in the 6 of 12 arrays that are shorter`},
		{`.[].employee.projects[].tasks[].details.completionDate`, `.completionDate: missing in 4 of the 12 objects here: null there`},
	} {
		ds := Check(c.line, s)
		if len(ds) != 1 || ds[0].Msg != c.msg {
			t.Errorf("%s\n  got  %+v\n  want %s", c.line, ds, c.msg)
		}
	}
}

// A key written twice (owner 2026-11-21): `"name": .name` is jq's {name}, as C# and JS let
// you leave the name out. A note on the entry, said when the cursor is on it, never drawn.
func TestShorthandNote(t *testing.T) {
	s, _ := shapes(t, "employees_5-level_10-KB_formatted.json")
	line := `.[].employee as $e | $e | {"name": .name, id: .id, did: .department.id, e: $e, "a b": ."a b", if: .if, n: .name.x}`
	var got []string
	rs := []rune(line)
	for _, d := range Check(line, s) {
		if d.Level == Note && strings.HasPrefix(d.Msg, "can be") {
			got = append(got, fmt.Sprintf("%s → %v", string(rs[d.From:d.To]), d.Suggest))
		}
	}
	want := `"name": .name → [name]; id: .id → [id]; e: $e → [$e]`
	if g := strings.Join(got, "; "); g != want {
		t.Errorf("got  %s\nwant %s", g, want)
	}
	if ds := Check(`.[].employee | {name: .name}`, s); len(ds) != 1 || ds[0].Msg != "can be just name: {name} is {name: .name}" {
		t.Errorf("message: %+v", ds)
	}
}
