package assist

import (
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/scope"
)

// Checks show on the line (Q-046): a field never there in the warning style, a key asked of
// an array in the error style, in place of the field's own colour.
func TestCheckedPaint(t *testing.T) {
	tag := func(name string) func(string) string {
		return func(s string) string { return "<" + name + ">" + s + "</>" }
	}
	p := Palette{
		Kinds:  map[jqsyntax.Kind]func(string) string{jqsyntax.Field: tag("f")},
		Checks: map[scope.Level]func(string) string{scope.Warning: tag("warn"), scope.Error: tag("err")},
	}
	paint := Checked(p, newChecker(employees(t)))
	for _, c := range []struct{ line, want string }{
		{`.[].employee | .projects[0].id`, `.[]<f>.employee</> | <f>.projects</>[0]<warn>.id</>`},
		{`.[].employee | .projects.x`, `.[]<f>.employee</> | <f>.projects</><err>.x</>`},
		{`.[].employee.projects[9]`, `.[]<f>.employee</><f>.projects</><warn>[9]</>`},
		{`.[].employee | .na`, `.[]<f>.employee</> | <f>.na</>`}, // a note isn't drawn
		{`:push .[].employee.zz`, `:push .[]<f>.employee</><warn>.zz</>`},
	} {
		if got := editor.Render([]rune(c.line), paint(c.line), -1); got != c.want {
			t.Errorf("%s\n  got  %s\n  want %s", c.line, got, c.want)
		}
	}
}

// The dim line says why, when the cursor is on what a check found.
func TestCheckHint(t *testing.T) {
	hint := CheckHint(newChecker(employees(t)))
	line := `.[].employee | {"eid": .id, "pid": .projects[0].id}`
	at := strings.LastIndex(line, ".id") + 2
	if h := hint(line, at); h != "⚠ .id: no such key in the 6 objects here, so jq gives null; did you mean .projectId?" {
		t.Errorf("on .id: %q", h)
	}
	if h := hint(line, 3); h != "" {
		t.Errorf("on .[]: %q", h)
	}
	if h := hint(".[].employee | .na", 18); h != "· .na: keys starting \"na\": name" {
		t.Errorf("typing .na: %q", h)
	}
}
