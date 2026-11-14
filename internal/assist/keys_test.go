package assist

import (
	"os"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
)

// employees is a session on the owner's employees document, with its tree, as the REPL has.
func employees(t *testing.T) *bench.Session {
	t.Helper()
	f, err := os.Open("../../testdata/employees_5-level_10-KB_formatted.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n, err := doc.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bench.New(n.Value())
	s.Doc = n
	return s
}

// Tab on keys (Q-007, DESIGN §22): the owner's line, a step at a time.
func TestCompleteKeys(t *testing.T) {
	complete := CompleteOn(employees(t))
	for _, c := range []struct {
		in, span string
		first    []string
	}{
		{".[].em▮", "em", []string{"employee"}},
		{".[].employee | .▮", "", []string{"id", "name", "position", "department", "projects"}},
		{`.[].employee | {"eid": .id, "pid": .projects[0].▮}`, "", []string{"projectId", "projectName", "startDate", "tasks"}},
		{`.[].employee | {"eid": .na▮}`, "na", []string{"name"}},
		{`.[].employee | .na▮me`, "name", []string{"name"}},              // the whole field goes
		{`.[].employee | .projects[0].id▮`, "id", []string{"projectId"}}, // anywhere in the key
		{`.[].employee | .projects[0].ID▮`, "ID", []string{"projectId"}}, // any case
		{`.[].employee | .p▮`, "p", []string{"position", "projects"}},
		{`.[].employee.department.manager.contact.▮`, "", []string{"email", "phone"}},
		{`.[].employee.projects | map(.ta▮`, "ta", []string{"tasks"}},
		{`.[].employee.projects[].tasks[] | select(.status == "Pending") | .details.▮`, "", []string{"hoursSpent", "technologiesUsed"}},
		{`.[].employee as $e | $e.department.▮`, "", []string{"id", "name", "manager"}},
		{`.[].employee | ."na▮`, `"na`, []string{`"name"`}},
		{`.[].employee | ."na▮"`, `"na"`, []string{`"name"`}},
		{`.[].employee | {na▮`, "na", []string{"name"}}, // shorthand {name}
		{`:push .[].em▮`, "em", []string{"employee"}},   // jq in a command
		// at an array, a key is asked of its elements: the [] forms
		{`.▮`, "", []string{"[]", "[0]", "[].employee"}},
		{`.em▮`, "em", []string{"[].employee"}},
		{`.[].employee as $e | $e.projects.▮`, ".", []string{"[]", "[0]", "[].projectId"}},
		{`.[].employee.projects.▮`, ".", []string{"[]", "[0]", "[].projectId", "[].projectName"}},
		{`.[].employee | .projects.ta▮`, ".ta", []string{"[].tasks"}},
		{`.[].employee | .projects.id▮`, ".id", []string{"[].projectId"}},
		{`.[].employee.projects[].tasks[].details.technologiesUsed.▮`, ".", []string{"[]", "[0]"}}, // strings: no keys
		{`.[].employee.name | .▮`, "", nil},                                                        // strings have no keys
		{`.[].employee | .zz▮`, "zz", nil},
		{`def f: .x▮`, "", nil},                    // . depends on the caller
		{`.[] | sel▮`, "sel", []string{"select("}}, // names, as before
	} {
		i := strings.Index(c.in, "▮")
		line := strings.Replace(c.in, "▮", "", 1)
		from, to, cs := complete(line, len([]rune(c.in[:i])))
		var ins []string
		for _, k := range cs {
			ins = append(ins, k.Insert)
		}
		if len(ins) > len(c.first) {
			ins = ins[:len(c.first)]
		}
		span := string([]rune(line)[from:to])
		if span != c.span || strings.Join(ins, " ") != strings.Join(c.first, " ") {
			t.Errorf("%s\n  span %q, %q; want %q, %q", c.in, span, ins, c.span, c.first)
		}
	}
}

// Each key's hint is what it holds, one level: the type shows as the key lands.
func TestCompleteKeyHints(t *testing.T) {
	complete := CompleteOn(employees(t))
	for _, c := range []struct{ in, hint string }{
		{".[].employee | .na▮", `name  string ×6  all distinct  e.g. "Jodi Reyes"`},
		{".[].employee | .proj▮", `projects  array ×6  len 1 of object ×6 {projectId projectName startDate tasks}`},
		{".▮", `[]  each element: object ×6 {employee}`},
		{".em▮", `[].employee  object ×6 {id name position department projects}`},
		{".[].employee.projects[].tasks[].details.compl▮", `completionDate  string ×8  all distinct  e.g. "2025-02-12"  missing in 4 objects`},
	} {
		i := strings.Index(c.in, "▮")
		line := strings.Replace(c.in, "▮", "", 1)
		_, _, cs := complete(line, len([]rune(c.in[:i])))
		if len(cs) == 0 || cs[0].Hint != c.hint {
			t.Errorf("%s\n  hints %+v\n  want %q", c.in, cs, c.hint)
		}
	}
}
