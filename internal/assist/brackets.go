package assist

import (
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// MatchBracket is ctrl+] (owner 2026-11-15, vim's % without its modes): on a bracket, or just
// after one, the cursor goes to its partner; elsewhere, back to the opener of the innermost
// bracket around it, closed or not, so in a half-typed `select(.a | len` it goes to select's (,
// and from an opener still open, to the end, where its closer is due.
// A string's \( and its ) count as a pair. Nothing to go to leaves the cursor where it is.
func MatchBracket(line string, cur int) (string, int) {
	off, expr, ok := jqPart(line)
	if !ok {
		return "", cur
	}
	c := cur - off
	var bs []jqsyntax.Token // the brackets, openers and closers, a \( as an opener
	for _, t := range jqsyntax.Lex(expr) {
		if t.Kind == jqsyntax.Bracket || t.Kind == jqsyntax.Interp {
			bs = append(bs, t)
		}
	}
	partner := make([]int, len(bs)) // each bracket's partner, by index; -1 for none
	var stack []int
	for i, t := range bs {
		partner[i] = -1
		if opens(t) {
			stack = append(stack, i)
		} else if n := len(stack); n > 0 {
			partner[i], partner[stack[n-1]] = stack[n-1], i
			stack = stack[:n-1]
		}
	}
	at := func(i int) int { return off + bs[i].Pos }
	for _, on := range []bool{true, false} { // on a bracket first, then just after one
		for i, t := range bs {
			if (on && t.Pos == c) || (!on && t.End == c) {
				if p := partner[i]; p >= 0 {
					return "", at(p)
				}
				if on && opens(t) { // still open: the end, where its closer is due
					return "", off + len([]rune(expr))
				}
			}
		}
	}
	for i := len(bs) - 1; i >= 0; i-- { // the innermost bracket around the cursor
		if opens(bs[i]) && bs[i].Pos < c && (partner[i] < 0 || bs[partner[i]].Pos >= c) {
			return "", at(i)
		}
	}
	return "", cur
}

func opens(t jqsyntax.Token) bool { return strings.ContainsAny(t.Text, "([{") }
