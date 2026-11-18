package jqsyntax

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

func TestLayout(t *testing.T) {
	for _, c := range []struct {
		in    string
		width int
		want  string
	}{
		// no group too wide: as written, spaces as one
		{"select(.age > 30)", 80, "select(.age > 30)"},
		{"  .a   +  .b ", 80, ".a + .b"},
		{"{team, names: [.people[].name]}", 80, "{team, names: [.people[].name]}"},
		{"reduce .[] as $x (0; . + $x)", 80, "reduce .[] as $x (0; . + $x)"},
		// a stage a line, at any width
		{".[] | .team", 80, ".[]\n| .team"},
		{".a|.b", 80, ".a\n| .b"},
		{".a, .b | .c", 80, ".a, .b\n| .c"},
		// a bracket with a pipeline stays shut when it fits (owner 2026-11-18)
		{"[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)", 80, "[.[].people[] | select(.age > 30)]\n| reduce .[].age as $i (0; . + $i)"},
		{". | length as $l | [range($l) as $i | .[$i].team]", 80, ".\n| length as $l\n| [range($l) as $i | .[$i].team]"},
		{"{a: (.x | f), b: 2}", 80, "{a: (.x | f), b: 2}"},
		// and opens when it doesn't: outermost first
		{"[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)", 30, "[\n    .[].people[]\n    | select(.age > 30)\n]\n| reduce .[].age as $i (\n    0;\n    . + $i\n)"},
		{". | length as $l | [range($l) as $i | .[$i].team]", 20, ".\n| length as $l\n| [\n    range($l) as $i\n    | .[$i].team\n]"},
		{"{a: (.x | f), b: 2}", 10, "{\n    a: (\n        .x\n        | f\n    ),\n    b: 2\n}"},
		{"[.a, (.b | .c)]", 10, "[\n    .a,\n    (\n        .b\n        | .c\n    )\n]"},
		{"map(.a | .b)", 10, "map(\n    .a\n    | .b\n)"},
		{"reduce .[] as $x (0; . + ($x | .n))", 20, "reduce .[] as $x (\n    0;\n    . + ($x | .n)\n)"},
		// commas only break inside an opened [ ] or { }
		{"f(.a, .b | .c)", 10, "f(\n    .a, .b\n    | .c\n)"},
		// a ( or [ holding one group hugs it
		{"map({a: .x, b: .y})", 10, "map({\n    a: .x,\n    b: .y\n})"},
		// a string's interpolation never opens
		{"\"x\\(.a | length)y\"", 4, "\"x\\(.a | length)y\""},
		{"[.a | \"\\(.b | .c)\"]", 10, "[\n    .a\n    | \"\\(.b | .c)\"\n]"},
		// a long line breaks before an operator, a step in
		{"{a: 1} + {b: 2} + {c: 3}", 20, "{a: 1} + {b: 2}\n    + {c: 3}"},
		{".a and .b or .c", 12, ".a and .b\n    or .c"},
		// if
		{"if .a then 1 else 2 end", 80, "if .a then 1 else 2 end"},
		{"[.[] | if . then 1 else 2 end]", 20, "[\n    .[]\n    | if . then\n        1\n    else\n        2\n    end\n]"},
		{"[.[] | if . then .a | .b else 2 end]", 20, "[\n    .[]\n    | if . then\n        .a\n        | .b\n    else\n        2\n    end\n]"},
		{"{if: 1, a: (.b | .c)}", 80, "{if: 1, a: (.b | .c)}"},
		// an if with an elif, or longer than ifWidth, opens, and so do the brackets around it (owner 2026-11-10)
		{"if .a then 1 elif .b then 2 else 3 end", 80, "if .a then\n    1\nelif .b then\n    2\nelse\n    3\nend"},
		{"map(if .age > 30 then {name, band: \"over 30\", team} else {name} end)", 80, "map(\n    if .age > 30 then\n        {name, band: \"over 30\", team}\n    else\n        {name}\n    end\n)"},
		{"if .a then \"short\" else \"also short\" end", 80, "if .a then \"short\" else \"also short\" end"},
		{"if .a then 1 elif .b then 2", 80, "if .a then 1 elif .b then 2"},
		// defs: one a line, opened after the colon when too wide
		{"def f: .a | .b; f", 80, "def f: .a | .b;\nf"},
		{"def f: .a | .b; f", 10, "def f:\n    .a\n    | .b;\nf"},
		{"def f: 1; f", 80, "def f: 1;\nf"},
		// comments: after code they stay there, on their own line they keep it; a blank line is kept
		{".a # first\n| .b", 80, ".a # first\n| .b"},
		{".a # trailing", 80, ".a # trailing"},
		{".a # c\n\n\n| .b", 80, ".a # c\n\n| .b"},
		{"{ # c\n a: 1}", 80, "{ # c\n    a: 1\n}"},
		{"# doc\ndef f: 1;\n\n# doc g\ndef g: 2; f", 80, "# doc\ndef f: 1;\n\n# doc g\ndef g: 2;\nf"},
		{".a\n# c\n| .b", 80, ".a\n# c\n| .b"},
		// |= is an update, not a stage
		{".a |= . + 1", 80, ".a |= . + 1"},
		// half typed
		{"[.a | ", 4, "[\n    .a\n    |"},
		{"[.a | select(", 4, "[\n    .a\n    | select("},
		{"\"a\\(.b | ", 4, "\"a\\(.b |"},
		{"[.a | .b))", 4, "[\n    .a\n    | .b))"},
		{"", 80, ""},
	} {
		if got := Layout(c.in, c.width); got != c.want {
			t.Errorf("Layout(%q, %d)\n got:\n%s\nwant:\n%s", c.in, c.width, got, c.want)
		}
	}
}

// layoutInvariants: at any width, the layout is the same tokens, so the same program; laying
// it out again changes nothing.
func layoutInvariants(t *testing.T, s string) {
	for _, w := range []int{Width, 12} {
		layoutAt(t, s, w)
	}
}

func layoutAt(t *testing.T, s string, width int) {
	got := Layout(s, width)
	if a, b := words(s), words(got); a != b {
		t.Fatalf("Layout(%q) changed the tokens:\n%s\n%s", s, a, b)
	}
	if again := Layout(got, width); again != got {
		t.Fatalf("Layout(%q) isn't stable:\n%s\nthen\n%s", s, got, again)
	}
	// and Join, F8's way back: the same tokens, stable
	joined := Join(got)
	if a, b := words(s), words(joined); a != b {
		t.Fatalf("Join(Layout(%q)) changed the tokens:\n%s\n%s", s, a, b)
	}
	if again := Join(joined); again != joined {
		t.Fatalf("Join(Layout(%q)) isn't stable:\n%q\nthen\n%q", s, joined, again)
	}
	if _, err := gojq.Parse(s); err == nil {
		if _, err := gojq.Parse(got); err != nil {
			t.Fatalf("Layout(%q) doesn't parse: %v\n%s", s, err, got)
		}
		if _, err := gojq.Parse(joined); err != nil {
			t.Fatalf("Join(Layout(%q)) doesn't parse: %v\n%s", s, err, joined)
		}
	}
}

// words are the tokens but spaces, as kind and text.
func words(s string) string {
	var b strings.Builder
	for _, t := range Lex(s) {
		if t.Kind != Space {
			b.WriteString("[" + t.Kind.String() + ":" + t.Text + "]")
		}
	}
	return b.String()
}

func FuzzLayout(f *testing.F) {
	for _, s := range []string{
		"[.[].people[] | select(.age > 30) | .age] | add",
		`{a: "x\(.b | "y\(.c)")", "k": [1, 2.5e3]} | .a //= $__loc__`,
		"reduce .[] as {a: $x} (0; . + ($x | .n)) | @csv",
		"def f(g): g | g; f(.a?) ?// [.b] | .. | try error catch .",
		"# c \\\n c\n.a | a::b",
		"if . then 1 | 2 elif 2 then 3 else 4 end",
		"label $out | foreach .[] as $i (0; .+$i; if . > 3 then ., break $out else . end)",
		"[.a | .b))", "(if . | . then", `"a\(.b | `,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if utf8.ValidString(s) && !strings.ContainsRune(s, 0) {
			layoutInvariants(t, s)
		}
	})
}

// Join puts laid-out text back on one line (Q-026, DESIGN §23): F8's way back from Layout.
func TestJoin(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{".a\n| .b", ".a | .b"},
		{"[\n    .[].people[]\n    | select(.age > 30)\n]\n| reduce .[].age as $i (0; . + $i)",
			"[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)"},
		{"{\n    a: (\n        .x\n        | f\n    ),\n    b: 2\n}", "{a: (.x | f), b: 2}"},
		{"reduce .[] as $x (\n    0;\n    . + $x\n)", "reduce .[] as $x (0; . + $x)"},
		{"def f: 1;\nf", "def f: 1; f"},
		// the owner's line, from F8 (2026-11-15)
		{"[\"name\",\"id\",\"department_id\"], (\n    .[].employee\n    | [.name, .id, .department.id]\n)\n| @csv",
			`["name","id","department_id"], (.[].employee | [.name, .id, .department.id]) | @csv`},
		// spaces on a line are the writer's, and a string's line break is its own
		{".a  +  .b\n| .c", ".a  +  .b | .c"},
		{"\"a\nb\"\n| .c", "\"a\nb\" | .c"},
		// a comment runs to its line's end, so its break stays
		{".a # first\n| .b\n| .c", ".a # first\n| .b | .c"},
		{"\n  .a\n\n| .b  \n", ".a | .b"},
		{".a", ".a"},
		{"", ""},
	} {
		if got := Join(c.in); got != c.want {
			t.Errorf("Join(%q)\n got: %q\nwant: %q", c.in, got, c.want)
		}
	}
}

// TestLayoutExamples: every examples/jq program laid out at Width (owner 2026-11-18, :e main
// then F8 ran past the terminal at twice the lines): the same program, no code line past the
// width (a comment can't be broken), and no more than half as many lines again as written.
func TestLayoutExamples(t *testing.T) {
	files, err := filepath.Glob("../../examples/jq/*/query.jq")
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := strings.ReplaceAll(string(b), "\r\n", "\n")
		layoutInvariants(t, src)
		got := Layout(src, Width)
		for _, l := range strings.Split(got, "\n") {
			if n := utf8.RuneCountInString(l); n > Width && !strings.Contains(l, "#") {
				t.Errorf("%s: a line of %d past %d:\n%s", f, n, Width, l)
			}
		}
		if n, was := strings.Count(got, "\n")+1, strings.Count(src, "\n")+1; n > was*2 {
			t.Errorf("%s: %d lines laid out, %d written", f, n, was)
		}
	}
}
