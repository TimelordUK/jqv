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
		// optional keys
		{`.[].employee.projects[].tasks[].details.completionDate`, `.completionDate note []`},
		// nothing to say
		{`.[].employee.projects[-1].projectId`, ``},
		{`.[].employee.junk?`, ``},
		{`.[].employee.projects[9]?`, ``},
		{`def f: .zz; .[].employee | map(.qq)`, ``}, // a def's body; map over an object's mixed values
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
