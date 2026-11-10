package assist

import (
	"fmt"
	"testing"

	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// markers is a palette that writes [kind:text], and brackets as [bN:text] by their colour's
// turn, so a painted line reads in a test.
func markers() Palette {
	p := Palette{Kinds: map[jqsyntax.Kind]func(string) string{}}
	for k := jqsyntax.Space; k <= jqsyntax.Error; k++ {
		if k == jqsyntax.Space {
			continue
		}
		p.Kinds[k] = func(s string) string { return "[" + k.String() + ":" + s + "]" }
	}
	for i := range 3 {
		p.Brackets = append(p.Brackets, func(s string) string { return fmt.Sprintf("[b%d:%s]", i, s) })
	}
	return p
}

func TestHighlight(t *testing.T) {
	paint := Highlight(markers())
	for _, c := range []struct{ in, want string }{
		{"select(.age > 30)", "[name:select][b0:(][field:.age] [op:>] [num:30][b0:)]"},
		// rainbow: a pair shares a colour, its neighbours take the next, round again after three
		{"[[[[1]]]]", "[b0:[][b1:[][b2:[][b0:[][num:1][b0:]][b2:]][b1:]][b0:]]"},
		{`"a\(.b)c"`, `[quote:"][text:a][b0:\(][field:.b][b0:)][text:c][quote:"]`},
		{"[.a)]", "[b0:[][field:.a][error:)][b0:]]"}, // closes nothing: an error, not a colour
		{":tree .a | .b", ":tree [field:.a] [pipe:|] [field:.b]"},
		{":help select", ":help select"}, // not jq: plain
		{".a # note", "[field:.a] [comment:# note]"},
	} {
		if got := editor.Render([]rune(c.in), paint(c.in), -1); got != c.want {
			t.Errorf("%q\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}
