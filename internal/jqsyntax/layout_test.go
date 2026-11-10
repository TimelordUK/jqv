package jqsyntax

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

func TestLayout(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// no pipeline: as written
		{"select(.age > 30)", "select(.age > 30)"},
		{"  .a   +  .b ", ".a + .b"},
		{"{team, names: [.people[].name]}", "{team, names: [.people[].name]}"},
		{"reduce .[] as $x (0; . + $x)", "reduce .[] as $x (0; . + $x)"},
		// a stage a line
		{".[] | .team", ".[]\n| .team"},
		{".a|.b", ".a\n| .b"},
		{"[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)",
			"[\n    .[].people[]\n    | select(.age > 30)\n]\n| reduce .[].age as $i (0; . + $i)"},
		{". | length as $l | [range($l) as $i | .[$i].team]",
			".\n| length as $l\n| [\n    range($l) as $i\n    | .[$i].team\n]"},
		// opened up because there's a pipeline somewhere inside
		{"{a: (.x | f), b: 2}", "{\n    a: (\n        .x\n        | f\n    ),\n    b: 2\n}"},
		{"[.a, (.b | .c)]", "[\n    .a,\n    (\n        .b\n        | .c\n    )\n]"},
		{"map(.a | .b)", "map(\n    .a\n    | .b\n)"},
		{"reduce .[] as $x (0; . + ($x | .n))",
			"reduce .[] as $x (\n    0;\n    . + (\n        $x\n        | .n\n    )\n)"},
		// commas only break inside an opened [ ] or { }
		{".a, .b | .c", ".a, .b\n| .c"},
		{"f(.a, .b | .c)", "f(\n    .a, .b\n    | .c\n)"},
		// a string's \(…) never opens
		{`"x\(.a | length)y"`, `"x\(.a | length)y"`},
		{`[.a | "\(.b | .c)"]`, "[\n    .a\n    | \"\\(.b | .c)\"\n]"},
		// if
		{"if .a then 1 else 2 end", "if .a then 1 else 2 end"},
		{"if .a then .b | .c elif .d then 2 else 3 end",
			"if .a then\n    .b\n    | .c\nelif .d then\n    2\nelse\n    3\nend"},
		{"[.[] | if . then 1 else 2 end]", "[\n    .[]\n    | if . then 1 else 2 end\n]"},
		{"[.[] | if . then .a | .b else 2 end]",
			"[\n    .[]\n    | if . then\n        .a\n        | .b\n    else\n        2\n    end\n]"},
		{"{if: 1, a: (.b | .c)}", "{\n    if: 1,\n    a: (\n        .b\n        | .c\n    )\n}"},
		// an if with an elif, or longer than ifWidth, opens with no pipeline in it, and opens
		// the brackets around it (owner 2026-11-10, from use)
		{"if .a then 1 elif .b then 2 else 3 end",
			"if .a then\n    1\nelif .b then\n    2\nelse\n    3\nend"},
		{`[.[].people[] | if .age > 30 then {name, band: "over 30"} elif .age > 26 then {name, band: "late 20s"} else {name, band: "young"} end]`,
			"[\n    .[].people[]\n    | if .age > 30 then\n        {name, band: \"over 30\"}\n    elif .age > 26 then\n        {name, band: \"late 20s\"}\n    else\n        {name, band: \"young\"}\n    end\n]"},
		{`if .age > 30 then {name, band: "over 30", team} else {name, band: "young"} end`,
			"if .age > 30 then\n    {name, band: \"over 30\", team}\nelse\n    {name, band: \"young\"}\nend"},
		{`map(if .age > 30 then {name, band: "over 30", team} else {name} end)`,
			"map(\n    if .age > 30 then\n        {name, band: \"over 30\", team}\n    else\n        {name}\n    end\n)"},
		{`if .a then "short" else "also short" end`, `if .a then "short" else "also short" end`},
		{"if .a then 1 elif .b then 2", "if .a then 1 elif .b then 2"}, // half typed: no end, not opened
		// defs and comments
		{"def f: .a | .b; f", "def f: .a\n| .b;\nf"},
		{"def f: 1; f", "def f: 1;\nf"},
		{".a # first\n| .b", ".a # first\n| .b"},
		{".a # trailing", ".a # trailing"},
		// |= is an update, not a stage
		{".a |= . + 1", ".a |= . + 1"},
		// half typed
		{"[.a | ", "[\n    .a\n    |"},
		{"[.a | select(", "[\n    .a\n    | select("},
		{`"a\(.b | `, `"a\(.b |`},
		{"[.a | .b))", "[\n    .a\n    | .b))"},
		{"", ""},
	} {
		if got := Layout(c.in); got != c.want {
			t.Errorf("Layout(%q)\n got:\n%s\nwant:\n%s", c.in, got, c.want)
		}
	}
}

// layoutInvariants: the layout is the same tokens, so the same program; laying it out again
// changes nothing.
func layoutInvariants(t *testing.T, s string) {
	got := Layout(s)
	if a, b := words(s), words(got); a != b {
		t.Fatalf("Layout(%q) changed the tokens:\n%s\n%s", s, a, b)
	}
	if again := Layout(got); again != got {
		t.Fatalf("Layout(%q) isn't stable:\n%s\nthen\n%s", s, got, again)
	}
	if _, err := gojq.Parse(s); err == nil {
		if _, err := gojq.Parse(got); err != nil {
			t.Fatalf("Layout(%q) doesn't parse: %v\n%s", s, err, got)
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
