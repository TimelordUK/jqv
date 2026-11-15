package assist

import (
	"strings"
	"testing"
)

// ctrl+] (owner 2026-11-15): ▮ is the cursor before, ^ where it goes.
func TestMatchBracket(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`select▮(.a | length > 3)`, `select(.a | length > 3^)`},                     // on an opener: its closer
		{`select(.a | length > 3▮)`, `select^(.a | length > 3)`},                     // on a closer: its opener
		{`select(.a | length > 3)▮`, `select^(.a | length > 3)`},                     // just after a closer
		{`.[] | select(.employee.name | len▮`, `.[] | select^(.employee.name | len`}, // inside, unclosed
		{`.[] | select▮(.employee.name | len`, `.[] | select(.employee.name | len^`}, // an opener still open: the end
		{`{a: [.b, (.c▮)]}`, `{a: [.b, ^(.c)]}`},
		{`{a: [.b, (.c)▮]}`, `{a: ^[.b, (.c)]}`}, // on ] (the cursor's block is on it) beats just after )
		{`{a: [.b, .c▮]}`, `{a: ^[.b, .c]}`},
		{`"x\(.a▮)"`, `"x^\(.a)"`},
		{`.a | .b▮`, `.a | .b^`},             // no bracket: stays
		{`:push map(.a▮)`, `:push map^(.a)`}, // a command's jq
		{`:help map(▮`, `:help map(^`},       // not jq
	} {
		i := strings.Index(c.line, "▮")
		line := c.line[:i] + c.line[i+len("▮"):]
		_, to := MatchBracket(line, len([]rune(c.line[:i])))
		rs := []rune(line)
		if got := string(rs[:to]) + "^" + string(rs[to:]); got != c.want {
			t.Errorf("%s\n  got  %s\n  want %s", c.line, got, c.want)
		}
	}
}
