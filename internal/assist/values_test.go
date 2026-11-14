package assist

import (
	"strings"
	"testing"
)

// Tab on values (R-006, DESIGN §22 step 6): the values seen where a comparison's left side
// points, or a call's input, most frequent first.
func TestCompleteValues(t *testing.T) {
	complete := CompleteOn(employees(t))
	const tasks = `.[].employee.projects[].tasks[] | `
	for _, c := range []struct {
		in, span string
		first    []string
	}{
		{tasks + `select(.status == ▮`, "", []string{`"Completed"`, `"In Progress"`, `"Pending"`}},
		{tasks + `select(.status ==▮`, "", []string{` "Completed"`}}, // a space before it
		{tasks + `select(.status == "P▮`, `"P`, []string{`"Pending"`}},
		{tasks + `select(.status == "pend▮"`, `"pend"`, []string{`"Pending"`}}, // any case, the closing quote too
		{tasks + `select(.status != "▮`, `"`, []string{`"Completed"`, `"In Progress"`, `"Pending"`}},
		{tasks + `select(.status | IN("Pending", "C▮`, `"C`, []string{`"Completed"`}},
		{tasks + `select(.details.hoursSpent > ▮`, "", []string{"29", "73"}},
		{tasks + `select(.details.hoursSpent > 6▮`, "6", []string{"62"}},
		{`.[].employee | select(.name == "Jo▮`, `"Jo`, []string{`"Jodi Reyes"`}},
		{`.[].employee | select(.name | startswith("A▮`, `"A`, []string{`"Ashley Mitchell"`, `"Adam Smith"`}},
		{`.[].employee as $e | select($e.position == "Sec▮`, `"Sec`, []string{`"Secretary, company"`}},
		{`.[].employee | select(.department.name == "zz▮`, `"zz`, nil},
		{`.[].employee | select(.department == ▮`, "", nil}, // objects: no values
		{`.[].employee | select(.name | length > ▮`, "", []string{"10"}},
		{`.[].employee | select((.projects | length) == ▮`, "", []string{"1"}},
		{`.[].employee | select(.position != "Pathologist" and .name == "A▮`, `"A`, []string{`"Ashley Mitchell"`, `"Adam Smith"`}}, // after and
		{`.[] | select(.x == ▮`, "", nil},                                                                                          // nothing seen there
		{`.[].employee | map(▮`, "", nil},                                                                                          // not a value
		{`.[].employee | select(.na▮`, "na", []string{"name"}},                                                                     // keys still
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
		if span != c.span || strings.Join(ins, " | ") != strings.Join(c.first, " | ") {
			t.Errorf("%s\n  span %q, %q; want %q, %q", c.in, span, ins, c.span, c.first)
		}
	}
}

func TestCompleteValueHints(t *testing.T) {
	complete := CompleteOn(employees(t))
	for _, c := range []struct{ in, hint string }{
		{`.[].employee.projects[].tasks[] | select(.status == ▮`, `"Completed"  ×8 of 12`},
		{`.[].employee.projects[].tasks[] | select(.details.hoursSpent > ▮`, `29  ×2 of 12  (numbers 13..73)`},
	} {
		i := strings.Index(c.in, "▮")
		line := strings.Replace(c.in, "▮", "", 1)
		_, _, cs := complete(line, len([]rune(c.in[:i])))
		if len(cs) == 0 || cs[0].Hint != c.hint {
			t.Errorf("%s\n  hints %+v\n  want %q", c.in, cs, c.hint)
		}
	}
}

// Tab never panics, anywhere in a line, half typed or not.
func TestCompleteAnywhere(t *testing.T) {
	complete := CompleteOn(employees(t))
	for _, line := range []string{
		`.[].employee | select(.status == "In \"Pro\(.x)gress" and .n != 1) | {a: .b, "c": [.d[0], $e.f]}`,
		`"`, `"\(`, `== "`, `(== `, `IN(`, `,"`, `.a[`, `."`, `{"`, `reduce .[] as $x (0; . + $x.`,
		`:push .[].employee | select(.name == "`, `def f: . == "a"; f`, `.a == 1 == 2`, `select(test("`,
	} {
		for cur := 0; cur <= len([]rune(line)); cur++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%q at %d: %v", line, cur, r)
					}
				}()
				complete(line, cur)
			}()
		}
	}
}
