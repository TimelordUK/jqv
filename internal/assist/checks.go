package assist

import (
	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/scope"
)

// checker keeps the last line's checks (Q-046, DESIGN §22), since the painter and the
// hinter both want them on every key and the line is usually the same for both.
type checker struct {
	s      *bench.Session
	line   string
	shapes *scope.Shapes
	diags  []scope.Diag // in the line's runes
}

func (c *checker) check(line string) []scope.Diag {
	sh := c.s.Shapes()
	if line == c.line && sh == c.shapes {
		return c.diags
	}
	c.line, c.shapes, c.diags = line, sh, nil
	off, expr, ok := jqPart(line)
	if !ok {
		return nil
	}
	for _, d := range scope.Check(expr, sh) {
		d.From, d.To = d.From+off, d.To+off
		c.diags = append(c.diags, d)
	}
	return c.diags
}

// Checked is the editor's Painter for jq with checks: the line coloured as Highlight does,
// then a field or index a check warns of drawn in p's style for its level (a note isn't
// drawn: it is said on the dim line when the cursor is on it).
func Checked(p Palette, c *checker) editor.Painter {
	plain := Highlight(p)
	return func(line string) []editor.Span {
		spans := plain(line)
		for _, d := range c.check(line) {
			style := p.Checks[d.Level]
			if style == nil {
				continue
			}
			// the check's span takes the place of the tokens' spans inside it, in order
			var out []editor.Span
			placed := false
			for _, sp := range spans {
				if sp.From >= d.From && sp.To <= d.To {
					continue
				}
				if !placed && sp.From >= d.To {
					out, placed = append(out, editor.Span{From: d.From, To: d.To, Style: style}), true
				}
				out = append(out, sp)
			}
			if !placed {
				out = append(out, editor.Span{From: d.From, To: d.To, Style: style})
			}
			spans = out
		}
		return spans
	}
}

// newChecker is a checker on s, for Checked and CheckHint to share.
func newChecker(s *bench.Session) *checker { return &checker{s: s} }

// CheckHint is the editor's Hinter: the reason for the check the cursor is on or just
// after, the most serious first, marked by its level.
func CheckHint(c *checker) func(line string, cur int) string {
	return func(line string, cur int) string {
		var best *scope.Diag
		for i, d := range c.check(line) {
			if d.From <= cur && cur <= d.To && (best == nil || d.Level > best.Level) {
				best = &c.diags[i]
			}
		}
		if best == nil {
			return ""
		}
		return mark[best.Level] + best.Msg
	}
}

var mark = map[scope.Level]string{scope.Error: "✗ ", scope.Warning: "⚠ ", scope.Note: "· "}
