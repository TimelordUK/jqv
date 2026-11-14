package assist

import (
	"strings"
	"testing"
)

// at splits a line at its ▮.
func at(s string) (string, int) {
	i := strings.Index(s, "▮")
	return strings.Replace(s, "▮", "", 1), len([]rune(s[:i]))
}

// The context line (Q-047): what . is at the cursor, as you type.
func TestContextHint(t *testing.T) {
	hint := ContextHint(employees(t))
	for _, c := range []struct{ in, want string }{
		{`.▮`, `.  array  len 6 of object ×6 {employee}`},
		{`.[].employee | .▮`, `.[].employee  object ×6 {id name position department projects}`},
		{`.[].employee | .projects▮`, `.[].employee.projects  array ×6  len 1 of object ×6 {projectId projectName startDate tasks}`},
		{`.[].employee | .projects[0].▮`, `.[].employee | .projects[0]  object ×6 {projectId projectName startDate tasks}`},
		{`.[].employee | .na▮`, `.[].employee  object ×6 {id name position department projects}`}, // not a key yet
		{`.[].employee | .name▮`, `.[].employee.name  string ×6  all distinct  e.g. "Jodi Reyes"`},
		{`.[].employee.projects[].tasks[] | select(.status == ▮`, `.[].employee.projects[].tasks[]  object ×12 {taskId title status details}`},
		{`.[].employee.projects[].tasks[] | select(.status == "Pending") | .▮`, `….tasks[] | select(.status == "Pending")  object ×2 {taskId title status details}`},
		{`.[].employee.zz▮`, `.[].employee  object ×6 {id name position department projects}`},
		{`range(1e9) | .▮`, `range(1e9)  number ×1000  all distinct  0..999  e.g. 0  (of the first outputs)`},
		{`def f: .x▮`, ``}, // . depends on the caller
		{`.a # x▮`, ``},    // a comment
		{`:help ma▮`, ``},  // a command
		{`▮`, ``},
	} {
		line, cur := at(c.in)
		if got := hint(line, cur); got != c.want {
			t.Errorf("%s\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

// A check at the cursor comes first; elsewhere the context line.
func TestHints(t *testing.T) {
	s := employees(t)
	hint := Hints(CheckHint(newChecker(s)), ContextHint(s))
	line, cur := at(`.[].employee | .projects[0].id▮`)
	if h := hint(line, cur); !strings.HasPrefix(h, "⚠ .id: no such key") {
		t.Errorf("on .id: %q", h)
	}
	line, cur = at(`.[].employee | .projects[0]▮.id`)
	if h := hint(line, cur); !strings.HasPrefix(h, ".[].employee | .projects[0]  object") {
		t.Errorf("before .id: %q", h)
	}
}

// F3 prints the stack at the cursor and keeps the line.
func TestStack(t *testing.T) {
	f3 := Stack(employees(t))
	line, cur := at(`.[].employee | {"pid": .projects[0].▮}`)
	out, moved := f3(line, cur)
	if moved != cur || !strings.Contains(out, "{…}") || !strings.Contains(out, "▮ .projects[0].") {
		t.Errorf("F3: cursor %d, %q", moved, out)
	}
	if out, _ := f3(":help", 5); !strings.HasPrefix(out, "F3 shows") {
		t.Errorf("F3 on a command: %q", out)
	}
}
