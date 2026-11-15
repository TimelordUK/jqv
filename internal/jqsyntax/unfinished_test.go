package jqsyntax

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

// What a line leaves open (Q-026, DESIGN §23), as `text@pos close` with `!stray` when a
// closer came that doesn't close it.
func TestUnfinished(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		// the owner's shapes
		{`.[] | select(.employee.name |`, `select(@6 ) |@28`},
		{`.[] | select(.employee.name`, `select(@6 )`},
		{`.[].employee | {a: .name`, `{@15 }`},
		{`.[].employee |`, `|@13`},
		{`.[] | select(.a) | map(`, `map(@19 )`},
		{`{a: [1, (2`, `{@0 } [@4 ] (@8 )`},
		// strings and interpolation
		{`"abc`, `"@0 "`},
		{`"a\(.b`, `"@0 " \(@2 )`},
		{`"a\(.b)c" | .`, ``},
		// keywords
		{`if . then 1`, `if@0 end`},
		{`if . then 1 else 2 end`, ``},
		{`def f: .a`, `def f@0 ;`},
		{`def f(a; b): a | b; f(1; 2)`, ``},
		{`def f: if . then 1 else 2 end; f`, ``},
		{`.[] as $x`, `as $x@4`},
		{`.[] as [$a, $b]`, `as [$a, $b]@4`},
		{`.a and`, `and@3`},
		{`.a ==`, `==@3`},
		{`.a,`, `,@2`},
		{`.a?`, ``},
		{`reduce .[] as $x`, `reduce@0 (…)`},
		{`reduce .[] as $x (0`, `(@17 )`},
		{`reduce (.[]) as $x (0; . + $x) | .`, ``},
		{`A.`, `.@1`},
		{`def f: 1;`, ``}, // a line of defs is for the context (Q-037), not unfinished
		// closed by the wrong thing
		{`select(.a]`, `select(@0 ) !9`},
		{`(if . then 1)`, `if@1 end !12`},
		{`.name)`, ``}, // closes nothing: jq's own message says so
		// comments don't count
		{".a | # more\n", `|@3`},
		{".a # (", ``},
	} {
		var got []string
		for _, o := range Unfinished(c.line) {
			s := fmt.Sprintf("%s@%d", o.Text, o.Pos)
			if o.Close != "" {
				s += " " + o.Close
			}
			if o.Stray >= 0 {
				s += fmt.Sprintf(" !%d", o.Stray)
			}
			got = append(got, s)
		}
		if g := strings.Join(got, " "); g != c.want {
			t.Errorf("%q\n  got  %s\n  want %s", c.line, g, c.want)
		}
	}
}

func TestExplain(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`.[] | select(.employee.name`, `select( at column 7 is still open: close it with )`},
		{`.[] | select(.employee.name |`, `| at the end wants a next stage; select( at column 7 is still open: close it with )`},
		{`{a: [1`, `still open: { at column 1, [ at column 5: close them with ] }`},
		{`.[] as $x`, `as $x at the end wants | and what uses $x`},
		{`select(.a]`, `select( at column 1 is closed by ] at column 10: write )`},
		{`(if . then 1)`, `if at column 2 has no end before the ) at column 13`},
		{`def f: .a`, `def f at column 1 wants a ; after its body`},
		{"def f:\n  .a | (", `def f at line 1 column 1 wants a ; after its body; ( at line 2 column 8 is still open: close it with )`},
	} {
		if g := Explain(c.line, Unfinished(c.line)); g != c.want {
			t.Errorf("%q\n  got  %s\n  want %s", c.line, g, c.want)
		}
	}
}

// Text gojq parses leaves nothing open, and text it stops on at the end (unexpected EOF)
// always leaves something: so enter can go on to the next line on exactly those.
func FuzzUnfinished(f *testing.F) {
	for _, s := range []string{
		"[.[].people[] | select(.age > 30) | .age] | add", `{a: "x\(.b | "y\(.c)")"}`,
		"reduce .[] as {a: $x} (0; . + $x) | @csv", "def f(g): g | g; f(.a?) ?// [.b]",
		"if . then 1 elif 2 then 3 else 4 end", ".a as [$x", `"a\(`, ".a | ", "select(.a]",
		"label $out | foreach .[] as $i (0; .+$i; if . > 3 then ., break $out else . end)",
	} {
		f.Add(s)
	}
	for k := range keywords { // each alone, and after a term: CI's fuzzing found `break`
		f.Add(k)
		f.Add(".a " + k)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return
		}
		os := Unfinished(s)
		_, err := gojq.Parse(s)
		var pe *gojq.ParseError
		switch {
		case err == nil && len(os) > 0:
			t.Fatalf("%q parses, but Unfinished gives %+v", s, os)
		case errors.As(err, &pe) && strings.Contains(err.Error(), "unexpected EOF") && len(os) == 0:
			t.Fatalf("%q: gojq wants more (%v), but Unfinished gives nothing", s, err)
		}
		Explain(s, os)
	})
}
