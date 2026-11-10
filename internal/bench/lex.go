package bench

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// lex is the :lex command (Q-034): how jqsyntax.Lex reads a line, as TestLex's cases are
// written, then the brackets with their depths (what rainbow colour goes by) and anything it
// couldn't read. It needn't parse: half-typed lines are what it is for.
func (s *Session) lex(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to show: run a line first, or :lex expr\n"
	}
	ts := jqsyntax.Lex(expr)
	var b strings.Builder
	b.WriteString(jqsyntax.Marks(ts) + "\n")
	var brackets, errs []string
	for _, t := range ts {
		switch t.Kind {
		case jqsyntax.Bracket, jqsyntax.Interp:
			brackets = append(brackets, fmt.Sprintf("%s%d", t.Text, t.Depth))
		case jqsyntax.Error:
			errs = append(errs, fmt.Sprintf("%q at %d", t.Text, t.Pos))
		}
	}
	if len(brackets) > 0 {
		b.WriteString("depth   " + strings.Join(brackets, " ") + "\n")
	}
	if len(errs) > 0 {
		b.WriteString("errors  " + strings.Join(errs, ", ") + "\n")
	}
	return b.String()
}
