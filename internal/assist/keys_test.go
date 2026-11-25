package assist

import (
	"fmt"
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

// Tab after :e offers the loaded program's defs, named stages and main.
func TestCompleteEditNames(t *testing.T) {
	s := employees(t)
	s.Do(":load ../../examples/jq/team-report/query.jq")
	complete := CompleteOn(s)
	from, to, cs := complete(":e ", 3)
	var names []string
	for _, c := range cs {
		names = append(names, c.Insert)
	}
	if from != 3 || to != 3 || strings.Join(names, " ") != "header people older avg $min $all $over main" {
		t.Errorf(":e Tab: %d..%d %v", from, to, names)
	}
	if _, _, cs := complete(":e o", 4); len(cs) != 1 || cs[0].Insert != "older" || cs[0].Hint != "def older($age), lines 20-21" {
		t.Errorf(":e o Tab: %+v", cs)
	}
}

// Tab in an object literal (Q-061, Q-062): the keys it has already are left out, and where no
// key is typed yet the first offer is every key left, written in as shorthand.
func TestCompleteObjectLiteral(t *testing.T) {
	complete := CompleteOn(employees(t))
	for _, c := range []struct {
		in, span string
		want     []string
	}{
		{`.[].employee | { id, i▮ }`, "i", []string{"position"}},
		{`.[].employee | { id, n▮, name }`, "n", []string{"position", "department"}},
		{`.[].employee | {▮}`, "", []string{"id", "name", "position", "department", "projects", "id, name, position, department, projects"}},
		{`.[].employee | { id, ▮`, "", []string{"name", "position", "department", "projects", "name, position, department, projects"}},
		{`.[].employee | { id,▮ }`, "", []string{"name", "position", "department", "projects", "name, position, department, projects"}}, // no space after the comma
		{`.[].employee | {"name": 1, $__loc__, ▮}`, "", []string{"id", "position", "department", "projects", "id, position, department, projects"}},
		{`.[].employee | {"id", (.x): 1, ▮`, "", []string{"name", "position", "department", "projects", "name, position, department, projects"}},
		{`.[].employee | {▮, name, position}`, "", []string{"id", "department", "projects", "id, department, projects"}},
		{`.[].employee | {id, name, position, department, projects, ▮}`, "", nil}, // all there
		{`.[].employee | {id, name, position, department, projects, p▮}`, "p", nil},
		{`.[].employee | {id, d: .department | {▮}}`, "", []string{"id", "name", "manager", "id, name, manager"}}, // its own object's keys only
		{`.[].employee | {i▮`, "i", []string{"id", "position"}},                                                   // typing: no every-key
	} {
		i := strings.Index(c.in, "▮")
		line := strings.Replace(c.in, "▮", "", 1)
		from, to, cs := complete(line, len([]rune(c.in[:i])))
		var ins []string
		for _, k := range cs {
			ins = append(ins, k.Insert)
		}
		span := string([]rune(line)[from:to])
		if span != c.span || strings.Join(ins, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s\n  span %q, %q\n  want %q, %q", c.in, span, ins, c.span, c.want)
		}
	}
}

// Every key is capped for wide objects, says so, and quotes keys that aren't plain names.
func TestCompleteEveryKey(t *testing.T) {
	m := map[string]any{"a b": 1, "if": 2}
	for i := range 30 {
		m[fmt.Sprintf("k%02d", i)] = i
	}
	s := bench.New(m)
	_, _, cs := CompleteOn(s)("{}", 1)
	if len(cs) == 0 {
		t.Fatal("no candidates")
	}
	every := cs[len(cs)-1]
	ins := strings.Split(every.Insert, ", ")
	if len(ins) != 20 || !strings.Contains(every.Hint, "first 20 of 32 keys") {
		t.Errorf("capped: %d keys, hint %q", len(ins), every.Hint)
	}
	if !strings.HasSuffix(cs[0].Hint, "shift+tab: every key") {
		t.Errorf("a key's hint says where every key is: %q", cs[0].Hint)
	}
	_, _, cs = CompleteOn(bench.New(map[string]any{"a b": 1, "if": 2, "x": 3}))("{}", 1)
	if len(cs) == 0 || cs[len(cs)-1].Insert != `"a b", "if", x` {
		t.Errorf("quoted: %+v", cs)
	}
}

// After `did: .department.id, ` Tab carries on from that entry (Q-079): .department's other
// keys come first, named with the entry's own prefix, and the last offer takes them all.
func TestCompleteSiblings(t *testing.T) {
	complete := CompleteOn(employees(t))
	const emp = `.[].employee | {id, name, `
	for _, c := range []struct {
		in   string
		want []string
	}{
		{emp + `did: .department.id, ▮}`, []string{
			"dname: .department.name", "dmanager: .department.manager",
			"position", "department", "projects", "position, department, projects",
			"dname: .department.name, dmanager: .department.manager"}},
		{emp + `department_id: .department.id, ▮`, []string{
			"department_name: .department.name", "department_manager: .department.manager"}},
		{emp + `departmentId: .department.id, ▮`, []string{
			"departmentName: .department.name", "departmentManager: .department.manager"}},
		{emp + `x: .department.id, ▮`, []string{ // no prefix to see: the key's own name
			"manager: .department.manager"}}, // name is in the literal already
		{emp + `did: .department.id, dname: .department.name, ▮`, []string{
			"dmanager: .department.manager", "position"}}, // one left: no take-all
		{emp + `dmn: .department.manager.name, ▮`, []string{ // initials: carried on as initials
			"dmi: .department.manager.id", "dmc: .department.manager.contact", "position"}},
		{emp + `dn: .department.name, ▮`, []string{
			"di: .department.id", "dm: .department.manager"}},
		{`.[].employee | {pn: .projects[0].projectName, ▮`, []string{"id"}}, // an index: not a path
		{emp + `did: .department.id, p▮`, []string{"position", "projects"}}, // typing: keys as before
		{emp + `did: (.department.id), ▮`, []string{"position"}},            // not a path
	} {
		i := strings.Index(c.in, "▮")
		line := strings.Replace(c.in, "▮", "", 1)
		_, _, cs := complete(line, len([]rune(c.in[:i])))
		var ins []string
		for _, k := range cs {
			ins = append(ins, k.Insert)
		}
		if c.in[i:] == "▮}" { // the whole list
			if strings.Join(ins, "|") != strings.Join(c.want, "|") {
				t.Errorf("%s\n  got  %q\n  want %q", c.in, ins, c.want)
			}
			if h := cs[0].Hint; !strings.HasSuffix(h, "shift+tab: the rest of .department") {
				t.Errorf("hint %q", h)
			}
			continue
		}
		if len(ins) > len(c.want) {
			ins = ins[:len(c.want)]
		}
		if strings.Join(ins, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s\n  got  %q\n  want %q", c.in, ins, c.want)
		}
	}
}
