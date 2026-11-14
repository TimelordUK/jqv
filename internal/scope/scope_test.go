package scope

import (
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/shape"
	"github.com/itchyny/gojq"
)

// cursor splits a line at its ▮.
func cursor(t *testing.T, s string) (string, int) {
	t.Helper()
	i := strings.Index(s, "▮")
	if i < 0 {
		t.Fatalf("no ▮ in %q", s)
	}
	return s[:i] + s[i+len("▮"):], len([]rune(s[:i]))
}

func TestAt(t *testing.T) {
	cases := []struct {
		line  string
		input string // "?" when not known
		term  string // Head|Path|Key, then + when a key is being typed
		value string // what the path gives before the key; "" to skip
	}{
		// the owner's line (DESIGN §22)
		{`.[].em▮`, `.`, `|.[]|em+`, `.[]`},
		{`.[].employee | .▮`, `.[].employee`, `||+`, `.[].employee`},
		{`.[].employee | {"eid": .id, "pid": .projects[0].▮}`, `.[].employee`, `|.projects[0]|+`, `.[].employee | .projects[0]`},
		{`.[].employee | {"eid": .i▮d, "pid": 1}`, `.[].employee`, `||i+`, ``},
		{`.[].employee | {"eid": .id, ▮`, `.[].employee`, `||`, ``},
		{`.[].employee | .projects▮`, `.[].employee`, `||projects+`, ``},
		{`.[].employee | .projects[]▮`, `.[].employee`, `|.projects[]|`, ``},
		{`.a | .b | .c▮`, `.a | .b`, `||c+`, ``},
		// brackets
		{`.[] | [.x, .y▮`, `.[]`, `||y+`, ``},
		{`.[] | (.x | .y▮`, `.[] | .x`, `||y+`, ``},
		{`.a[.b▮`, `.`, `||b+`, ``},
		{`.[] | "id: \(.i▮`, `.[]`, `||i+`, ``},
		// calls
		{`.[].employee.projects | map(.ta▮`, `.[].employee.projects | .[]`, `||ta+`, ``},
		{`map(select(.status == "x") | .de▮`, `.[] | select(.status == "x")`, `||de+`, ``},
		{`.[] | select(.na▮`, `.[]`, `||na+`, ``},
		{`.[] | select(.name == ▮`, `.[]`, `||`, ``},
		{`with_entries(.▮`, `to_entries[]`, `||+`, ``},
		{`any(.[]; .a▮`, `.[]`, `||a+`, ``},
		{`any(.a▮`, `.[]`, `||a+`, ``},
		{`INDEX(.items[]; .i▮`, `.items[]`, `||i+`, ``},
		{`limit(3; .[] | .x▮`, `.[]`, `||x+`, ``},
		{`sub("a"; .x▮`, `?`, `||x+`, ``},
		// bindings
		{`.[] as $e | $e.em▮`, `.`, `$e||em+`, `.[]`},
		{`.[] as $e | .x | select(. == $e) | .▮`, `.[] as $e | .x | select(. == $e)`, `||+`, ``},
		{`.[] as $e | .x | .▮`, `.x`, `||+`, ``},
		{`.[] as [$a, $b] | $b.▮`, `.`, `$b||+`, `.[] as [$a, $b] | $b`},
		{`$root.a.▮`, `.`, `$root|.a|+`, `$root | .a`},
		{`reduce .[] as $x (0; . + $x.▮`, `0`, `$x||+`, `.[]`},
		{`reduce .[] as $x (0; . + $x | .▮`, `.[] as $x | 0 | . + $x`, `||+`, ``},
		{`reduce .[] as $x ({}; .[$x.k▮`, `{}`, `$x||k+`, ``},
		{`.a |= map(.b▮`, `.a | .[]`, `||b+`, ``},
		{`.a |= .b▮`, `.a`, `||b+`, ``},
		{`.a += .b▮`, `.`, `||b+`, ``},
		// if, def
		{`.[] | if .a then .b | .c▮`, `.[] | .b`, `||c+`, ``},
		{`.[] | if .a then 1 else 2 end | .▮`, `.[] | if .a then 1 else 2 end`, `||+`, ``},
		{`def f: .x▮`, `?`, `||x+`, ``},
		{`def f: .x; .[] | .y▮`, `.[]`, `||y+`, ``},
		{`def f(g): g; f(.a▮`, `.`, `||a+`, ``},
		// paths
		{`.a."b c".▮`, `.`, `|.a."b c"|+`, `.a."b c"`},
		{`.a."pa▮`, `.`, `|.a|pa+`, ``},
		{`.a[1:3][]?.▮`, `.`, `|.a[1:3][]?|+`, ``},
		{`(.a | .b).▮`, `.`, `(.a | .b)||+`, `(.a | .b)`},
		{`[.a][0].▮`, `.`, `[.a]|[0]|+`, `[.a][0]`},
		{`.. | .na▮`, `..`, `||na+`, ``},
		{`..▮`, `.`, `..||`, ``},
		{`first(.[]).▮`, `.`, `first(.[])||+`, ``},
		{`1 + ▮`, `.`, `||`, ``},
		{`▮`, `.`, `||`, ``},
	}
	for _, c := range cases {
		line, cur := cursor(t, c.line)
		s := At(line, cur)
		in, ok := s.Input()
		if !ok {
			in = "?"
		}
		tm := s.Term
		got := tm.Head + "|" + tm.Path + "|" + tm.Key
		if tm.Typing {
			got += "+"
		}
		if in != c.input || got != c.term {
			t.Errorf("%s\n  input %q term %q, want %q %q", c.line, in, got, c.input, c.term)
			continue
		}
		if c.value != "" {
			if v, _ := s.Value(); v != c.value {
				t.Errorf("%s\n  value %q, want %q", c.line, v, c.value)
			}
		}
	}
}

// FuzzAt: At never panics, on any line at any cursor; and on a line gojq parses, every
// program it gives (the input, each frame's, each variable's, the term's value) parses too,
// so whoever runs them only meets the errors of running.
func FuzzAt(f *testing.F) {
	for _, s := range []string{
		`.[].employee | {"eid": .id, "pid": .projects[0].id}`,
		`.[] as $e | .x | select(. == $e) | $e.y`,
		`.[] as [$a, $b] | $b.c`,
		`reduce .[] as $x (0; . + $x.n | .)`,
		`foreach .[] as {a: $v} (0; . + $v; [., $v])`,
		`.a |= map(.b) | with_entries(.value |= tostring)`,
		`def f(g): g | .x; .[] | if .a then f(.b) elif .c then 1 else 2 end`,
		`label $out | .[] | "id: \(.i)" | try error catch .`,
		`.a."b c"[1:3][]?.d, (.a | .b).c, [.a][0].b, ..|.y?`,
		`{a, $__loc__, "b": .c | .d, (.e): 1} | any(.[]; .a) | INDEX(.i)`,
		`sub("(?<x>a)"; "\(.x)") | $root.a`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		parses := func(p string) bool { _, err := gojq.Parse(p); return err == nil }
		// gojq takes a NUL as the end of the text, where the lexer calls it an Error (as FuzzLex)
		whole := parses(line) && !strings.ContainsRune(line, 0)
		// Check never panics either: walked on a small shape, with runs giving nothing
		doc := map[string]any{"a": []any{map[string]any{"b": 1.0}, "s"}, "c": nil}
		Check(line, &Shapes{Input: shape.FromValues([]any{doc}, nil)})
		for cur := 0; cur <= len([]rune(line)); cur++ {
			s := At(line, cur)
			if !whole {
				continue
			}
			var ps []string
			if in, ok := s.Input(); ok {
				ps = append(ps, in)
			}
			for _, fr := range s.Frames {
				if fr.Input != "" {
					ps = append(ps, fr.Input)
				}
			}
			for _, v := range s.Vars {
				ps = append(ps, v)
			}
			if v, ok := s.Value(); ok {
				ps = append(ps, v)
			}
			for _, p := range ps {
				if !parses(p) {
					t.Fatalf("%q at %d: %q doesn't parse", line, cur, p)
				}
			}
		}
	})
}

// Frames are the stack: for the owner's line, the line's input, the stage, the object.
func TestFrames(t *testing.T) {
	line, cur := cursor(t, `.[].employee | {"eid": .id, "pid": .projects[0].▮}`)
	var got []string
	for _, f := range At(line, cur).Frames {
		got = append(got, f.Label+" = "+f.Input)
	}
	want := []string{". = .", ".[].employee = .[].employee", "{…} = .[].employee"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("frames\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	line, cur = cursor(t, `.[] | map(select(.a) | .▮)`)
	got = nil
	for _, f := range At(line, cur).Frames {
		got = append(got, f.Label+" = "+f.Input)
	}
	want = []string{". = .", ".[] = .[]", "map(▮) = .[] | .[]", "select(.a) = .[] | .[] | select(.a)"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("frames\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
