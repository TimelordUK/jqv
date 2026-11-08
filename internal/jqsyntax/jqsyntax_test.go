package jqsyntax

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// at splits a case written with ▮ at the cursor.
func at(s string) (string, int) {
	i := strings.Index(s, "▮")
	return strings.Replace(s, "▮", "", 1), len([]rune(s[:i]))
}

func TestUpTo(t *testing.T) {
	const q = "[.[].people[] | select(.age > 30) | .age] | reduce .[] as $item (0; . + $item)"
	for _, c := range []struct{ in, want string }{
		{"▮" + q, "[.[].people[] | select(.age > 30) | .age]"}, // before the [: its whole stage
		{"[.[].peo▮ple[] | select(.age > 30) | .age] | add", "[.[].people[]]"},
		{"[.[].people[] | sel▮ect(.age > 30) | .age] | add", "[.[].people[] | select(.age > 30)]"},
		{"[.[].people[] | select(.age▮ > 30) | .age] | add", "[.[].people[] | select(.age > 30)]"},
		{"[.[].people[] | select(.age > 30) | .age▮] | add", "[.[].people[] | select(.age > 30) | .age]"},
		{"[.[].people[] | select(.age > 30) | .age]▮ | add", "[.[].people[] | select(.age > 30) | .age]"},
		{q + "▮", q},
		{"[.[].people[] | select(.age > 30) | .age] | reduce .[] as $item (0; . + $it▮em)", q},
		{".a ▮| .b", ".a"},
		{".a |▮ .b", ".a | .b"},
		{".a | ▮", ".a"},
		{"▮", "."},
		{".a | map(.b▮ | .c) | length", ".a | map(.b)"},
		{"{name: .n▮ame, age} | .age", "{name: .name}"},
		{"if .a then .b▮ else .c end | .d", "if .a then .b end"},
		{"if .a then .b else .c end▮ | .d", "if .a then .b else .c end"},
		{`.a | "x|y\(.b▮ | .c)z" | length`, `.a | "x|y\(.b)"`},
		{`.a | "x|▮y" | length`, `.a | "x|y"`},
		{".a |= .+1▮ | .b", ".a |= .+1"},
		{".a # a | comment\n| .b▮", ".a # a | comment\n| .b"},
		{".end | .b▮", ".end | .b"},
		{"try .a▮ catch .b", "try .a"},
		{".a as $x | .b▮ | .c", ".a as $x | .b"},
		{".a as [$x, $y] | .b▮ | .c", ".a as [$x, $y] | .b"},
		{"reduce (.[] | .n▮) as $n (0; . + $n) | . * 2", "reduce (.[] | .n) as $n (0; . + $n)"},
	} {
		in, cur := at(c.in)
		if got := UpTo(in, cur); got != c.want {
			t.Errorf("UpTo(%q, %d)\n got  %q\n want %q", in, cur, got, c.want)
		}
	}
}

func TestStops(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string // UpTo at each stop
	}{
		{"[.[].people[] | select(.age > 30) | .age] | reduce .[] as $item (0; . + $item)", []string{
			"[.[].people[]]",
			"[.[].people[] | select(.age > 30)]",
			"[.[].people[] | select(.age > 30) | .age]",
			"[.[].people[] | select(.age > 30) | .age] | reduce .[] as $item (0; . + $item)",
		}},
		{".a", []string{".a"}},
		{"", nil},
		{".a | map(.b) | add", []string{".a", ".a | map(.b)", ".a | map(.b) | add"}},
	} {
		var got []string
		for _, s := range Stops(c.in) {
			got = append(got, UpTo(c.in, s))
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("Stops(%q):\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

// TestStopsParse checks that stepping along real queries always gives something gojq parses.
func TestStopsParse(t *testing.T) {
	for _, q := range []string{
		`.[].people[] | select(.age > 30 and .name != "x|y") | {name, age} | .name`,
		`[.orders[] | {id, total: (.items | map(.qty * .price) | add)}] | sort_by(.total) | reverse`,
		`.orders | group_by(.symbol) | map({key: .[0].symbol, count: length}) | from_entries`,
		`to_entries | map(select(.value | type == "number")) | from_entries`,
		`.a as $x | [.b[] | . + $x] | if length > 2 then .[:2] else . end | add`,
		`"\(.a | ascii_upcase)-\(.b)" | split("-") | .[0]`,
		`reduce (.[] | .n) as $n (0; . + $n) | . * 2`,
		`[paths(type == "number")] | map(join(".")) | .[]`,
	} {
		if _, err := gojq.Parse(q); err != nil {
			t.Fatalf("%q doesn't parse: %v", q, err)
		}
		for _, s := range Stops(q) {
			if p := UpTo(q, s); !parses(p) {
				t.Errorf("%q at %d: %q doesn't parse", q, s, p)
			}
		}
	}
}

// FuzzUpTo: never a panic, and whatever the cursor, the cut of a query that parses is at most
// as long as the query plus its closers.
func FuzzUpTo(f *testing.F) {
	for _, s := range []string{".a | .b", `[.a | "\(.b | .c)"] | add`, "if . then 1 else 2 end", "{a: (.b | .c)}"} {
		f.Add(s, 3)
	}
	f.Fuzz(func(t *testing.T, s string, cur int) {
		UpTo(s, cur)
		for _, st := range Stops(s) {
			if st < 0 || st > len([]rune(s)) {
				t.Fatalf("stop %d outside %q", st, s)
			}
			UpTo(s, st)
		}
	})
}
