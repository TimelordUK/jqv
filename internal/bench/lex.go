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

// ast is the :ast command (Q-048): the tree jqsyntax.Parse makes of the last line (or expr), a
// node a line, indented under its parent, with the text it spans when that says more than its
// name. Half-typed lines show where something is still to come.
func (s *Session) ast(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to show: run a line first, or :ast expr\n"
	}
	rs := []rune(expr)
	var b strings.Builder
	var walk func(n *jqsyntax.Node, depth int)
	walk = func(n *jqsyntax.Node, depth int) {
		label := n.Label()
		if n.Kind == jqsyntax.NBad {
			label = "bad"
		}
		b.WriteString(strings.Repeat("  ", depth) + label)
		switch span := strings.Join(strings.Fields(string(rs[n.Pos:n.End])), " "); {
		case n.Kind == jqsyntax.NMissing:
			b.WriteString("  … still to come")
		case n.Kind == jqsyntax.NBad:
			fmt.Fprintf(&b, "  %q doesn't fit here", span)
		case span != n.Text && len(n.Kids) > 0:
			if r := []rune(span); len(r) > 48 {
				span = string(r[:47]) + "…"
			}
			b.WriteString("  " + span)
		}
		if n.Open {
			b.WriteString("  (open)")
		}
		b.WriteByte('\n')
		for _, k := range n.Kids {
			walk(k, depth+1)
		}
	}
	root := jqsyntax.Parse(expr)
	walk(root, 0)
	if !root.Whole() {
		b.WriteString("not whole: gojq won't run it as it is\n")
	}
	return b.String()
}

// layout is the :fmt command (Q-035): the last line (or expr) laid out over lines by its
// structure (jqsyntax.Layout), to read, coloured as the line is when the front end paints.
func (s *Session) layout(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to show: run a line first, or :fmt expr\n"
	}
	out := jqsyntax.Layout(expr, s.LayoutWidth())
	if s.Paint != nil {
		out = s.Paint(out)
	}
	return out + "\n"
}

// LayoutWidth is how wide F8 and :fmt lay jq out: the terminal's width less the prompt (an
// entry's lines all sit under one as wide), at most 100 so a wide terminal still gives lines
// to read, at least 40; jqsyntax.Width when the terminal's width is unknown.
func (s *Session) LayoutWidth() int {
	if s.Cols == nil || s.Cols() <= 0 {
		return jqsyntax.Width
	}
	return min(max(s.Cols()-len([]rune(s.Prompt()))-1, 40), 100)
}
