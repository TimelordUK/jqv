package assist

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/bench"
)

func session(t *testing.T) *bench.Session {
	t.Helper()
	b, err := os.ReadFile("../../testdata/people.json")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return bench.New(v)
}

func TestHelp(t *testing.T) {
	h := HelpFor(session(t))
	if out, cur := h(".[] | ltrim", 9); !strings.HasPrefix(out, "trim, ltrim, rtrim") || cur != 9 {
		t.Errorf("F1 on ltrim: %q, cursor %d", out, cur)
	}
	if out, _ := h("@csv", 1); !strings.Contains(out, "Format strings") {
		t.Errorf("F1 on @csv: %q", out)
	}
	if out, _ := h(". | ", 4); !strings.Contains(out, ":push") {
		t.Errorf("F1 on no word is the REPL's help: %q", out)
	}
}

// The line from daily use: the reduce fails, and F5 on select shows what reaches it.
const failing = `.[].people[] | select(.age > 30) | reduce .[] as $item (0; . + $item)`

func TestRunToCursor(t *testing.T) {
	s := session(t)
	r := RunToCursor(s)
	full := s.Do(failing).Text
	if !strings.Contains(full, "cannot add") {
		t.Skipf("people.json no longer makes the reduce fail: %q", full)
	}
	cur := strings.Index(failing, "select") + 2
	out, to := r(failing, cur)
	if to != cur {
		t.Errorf("F5 leaves the cursor: %d", to)
	}
	if !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") || strings.Contains(out, "cannot add") {
		t.Errorf("F5 on select runs up to it:\n%s", out)
	}
	if out, _ := r(":push "+failing, 6+cur); !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") {
		t.Errorf("F5 in :push's argument:\n%s", out)
	}
	if out, _ := r(":stages", 3); out != notJQ {
		t.Errorf("F5 on a command: %q", out)
	}
}

func TestStep(t *testing.T) {
	s := session(t)
	back, on := Step(s, -1), Step(s, +1)
	var seen []string
	cur := 0
	for range 5 {
		out, to := on(failing, cur)
		if out == "" {
			break
		}
		seen = append(seen, strings.SplitN(out, "\n", 2)[0])
		cur = to
	}
	want := []string{
		"▸ .[].people[]",
		"▸ .[].people[] | select(.age > 30)",
		"▸ " + failing,
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Errorf("ctrl+down along the line:\n got  %q\n want %q", seen, want)
	}
	if out, to := on(failing, cur); out != "" || to != cur {
		t.Errorf("past the last stage it stays: %q %d", out, to)
	}
	out, to := back(failing, cur)
	if !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") || to != strings.Index(failing, " | reduce") {
		t.Errorf("ctrl+up: %q at %d", out, to)
	}
}

// F8 (Q-026, owner 2026-11-15): the entry laid out over lines in the editor, and joined back
// onto one line by F8 again. ▮ is the cursor, which stays on the token it was on.
func TestToggleLayout(t *testing.T) {
	s := session(t)
	f8 := ToggleLayout(s)
	show := func(text string, cur int) string {
		r := []rune(text)
		return string(r[:cur]) + "▮" + string(r[cur:])
	}
	for _, c := range []struct{ in, want, note string }{
		// one line: a stage a line, a bracket opened only when too wide (owner 2026-11-18)
		{".a | [.b | .▮c]", ".a\n| [.b | .▮c]", ""},
		{"[.a | .▮b]", "[.a | .▮b]", "nothing to lay out"},
		{`[.[] | {name: .employee.name, id: .employee.id, dept: .employee.department.name, ▮x: 1}]`,
			"[\n    .[]\n    | {\n        name: .employee.name,\n        id: .employee.id,\n        dept: .employee.department.name,\n        ▮x: 1\n    }\n]", ""},
		// laid out: joined back
		{"▮.a\n| .b", "▮.a | .b", ""},
		// laid out another way (narrower): laid out again, not joined
		{"[\n    .a\n    | .▮b\n]", "[.a | .▮b]", ""},
		// written over lines by hand: laid out from its own text, a blank line kept
		{".a\n\n| .b | .▮c", ".a\n\n| .b\n| .▮c", ""},
		// the owner's line, 2026-11-15: the bracket fits now, so it stays shut
		{`["name","id"], (.[].employee | [.name, .id]) | @csv▮`,
			"[\"name\",\"id\"], (.[].employee | [.name, .id])\n| @csv▮", ""},
		// a comment keeps its line break: joined as far as it goes, then laid out again
		{".a # c\n| .b\n| .c▮", ".a # c\n| .b | .c▮", ""},
		{".a # c\n| .b | .c▮", ".a # c\n| .b\n| .c▮", ""},
		// a command's jq
		{":push .a|.b▮", ":push .a\n| .b▮", ""},
		// nothing to do
		{".a▮", ".a▮", "nothing to lay out"},
		{":help map▮", ":help map▮", "F8 lays out jq"},
		{"▮", "▮", "nothing run yet"},
	} {
		at := strings.Index(c.in, "▮")
		out, cur, note := f8(strings.Replace(c.in, "▮", "", 1), len([]rune(c.in[:at])))
		if show(out, cur) != c.want || !strings.HasPrefix(note, c.note) || (c.note == "") != (note == "") {
			t.Errorf("F8 on %q: %q, note %q; want %q, note %q", c.in, show(out, cur), note, c.want, c.note)
		}
	}
	s.Do(".[] | .team")
	if out, cur, _ := f8("", 0); out != ".[]\n| .team" || cur != len(out) {
		t.Errorf("F8 on an empty line brings the last line, laid out: %q", out)
	}
}

// Enter's Continues (Q-026): what an entry leaves open, or "" when enter should run it.
func TestUnfinished(t *testing.T) {
	for line, want := range map[string]string{
		`.[] | select(.employee.name |`: `open: select( · | at the end`,
		`.[].employee | {a: [.name`:     `open: { [`,
		".[].employee\n|":               `| at the end`,
		`.[] as $e`:                     `as $e at the end`,
		`.[].employee | .name`:          ``,
		`select(.a]`:                    ``, // a mistake: run it, so its hint says which
		`:push .a |`:                    `| at the end`,
		`:help select(`:                 ``, // not jq
		``:                              ``,
	} {
		if got := Unfinished(line); got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}

// F8 on a whole main out in the editor (owner 2026-11-18: :e main then F8 on fix-dictionary ran
// past the terminal at twice the lines): laid out within the width, not much longer than
// written, and F8 again joins it rather than laying it out a second way.
func TestToggleLayoutMain(t *testing.T) {
	s := session(t)
	s.Do(":load ../../examples/jq/fix-dictionary/query.jq")
	main := s.Do(":e main").Edit
	f8 := ToggleLayout(s)
	out, _, note := f8(main, 0)
	if note != "" {
		t.Fatalf("F8 on main: %s", note)
	}
	w := s.LayoutWidth()
	for _, l := range strings.Split(out, "\n") {
		if n := len([]rune(l)); n > w && !strings.Contains(l, "#") {
			t.Errorf("a line of %d past %d: %s", n, w, l)
		}
	}
	// written to about 100 columns with closers hung at line ends (`| from_entries) as $fields`),
	// so at 80 with each closer on its own line it is longer: 89 lines from 36 (the old layout
	// gave 132 for the whole file, past the terminal)
	if n, was := strings.Count(out, "\n")+1, strings.Count(main, "\n")+1; n > was*5/2 {
		t.Errorf("main laid out in %d lines, written in %d:\n%s", n, was, out)
	}
	if again, _, _ := f8(out, 0); again == out || strings.Count(again, "\n") >= strings.Count(out, "\n") {
		t.Errorf("F8 again should join what F8 laid out:\n%s", again)
	}
}
