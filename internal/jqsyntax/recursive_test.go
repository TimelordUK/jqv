package jqsyntax

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// marked is text with each recursive def's name and its self-calls in «», and the builtins
// that recurse in ‹›, to read what Recursive found.
func marked(text string) string {
	rs := []rune(text)
	type mark struct {
		at   int
		open bool
		s    string
	}
	var ms []mark
	add := func(from, to int, o, c string) { ms = append(ms, mark{from, true, o}, mark{to, false, c}) }
	r := Recursive(text)
	for _, d := range r.Defs {
		add(d.Name[0], d.Name[1], "«", "»")
		for _, c := range d.Calls {
			add(c[0], c[1], "«", "»")
		}
	}
	for _, b := range r.Builtins {
		add(b[0], b[1], "‹", "›")
	}
	slices.SortStableFunc(ms, func(a, b mark) int {
		if a.at != b.at {
			return a.at - b.at
		}
		if a.open == b.open {
			return 0
		}
		if a.open {
			return 1 // a close before an open at the same place
		}
		return -1
	})
	var b strings.Builder
	at := 0
	for _, m := range ms {
		b.WriteString(string(rs[at:m.at]))
		b.WriteString(m.s)
		at = m.at
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// Recursion shown (Q-052): a def that calls itself, by jq's scoping (visible in its own body
// and after it; shadowed by an inner def or a closure parameter of the same name and arity),
// and the builtins that recurse.
func TestRecursive(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"def f: f; f", "def «f»: «f»; f"},
		{"def f($d): if $d > 2 then 1 else f($d + 1) end; f(0)", "def «f»($d): if $d > 2 then 1 else «f»($d + 1) end; f(0)"},
		{"def f: 1; f", "def f: 1; f"},                                 // a call after it isn't recursion
		{"def f: def f: 3; f; f", "def f: def f: 3; f; f"},             // the inner f is called, not itself
		{"def f(f): f; 1", "def f(f): f; 1"},                           // the parameter f, not the def
		{"def f: f(1); def f(x): x; 1", "def f: f(1); def f(x): x; 1"}, // another arity, and not yet defined
		{"def f(x): f; def f: f(1); 1", "def f(x): f; def f: f(1); 1"}, // each calls the other arity
		{"def g: def h: g; h; 1", "def «g»: def h: «g»; h; 1"},         // through an inner def
		{"recurse(.a) | .. | walk(.) | recurse", "‹recurse›(.a) | ‹..› | ‹walk›(.) | ‹recurse›"},
		{"def recurse: 1; recurse", "def recurse: 1; recurse"}, // the user's own, which doesn't
		{"[.[] | ", "[.[] | "},
		{"def f: f", "def «f»: «f»"}, // unfinished still found
	} {
		if got := marked(c.in); got != c.want {
			t.Errorf("%s\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

// The examples' recursive defs: org-tree's tree, org-chain's chains, fix-message's show and
// fix-dictionary's members; and org-below's recurse.
func TestRecursiveExamples(t *testing.T) {
	want := map[string][]string{
		"org-tree":       {"tree"},
		"org-chain":      {"chains"},
		"fix-message":    {"show"},
		"fix-dictionary": {"members"},
		"team-report":    nil,
	}
	for ex, names := range want {
		b, err := os.ReadFile(filepath.Join("..", "..", "examples", "jq", ex, "query.jq"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		var got []string
		for _, d := range Recursive(text).Defs {
			got = append(got, d.Def.Text)
			if len(d.Calls) == 0 {
				t.Errorf("%s: %s with no calls", ex, d.Def.Text)
			}
		}
		if !slices.Equal(got, names) {
			t.Errorf("%s: %v, want %v", ex, got, names)
		}
	}
	b, _ := os.ReadFile(filepath.Join("..", "..", "examples", "jq", "org-below", "query.jq"))
	if len(Recursive(string(b)).Builtins) != 1 {
		t.Errorf("org-below: recurse not found")
	}
}
