package jqsyntax

import "strings"

// Join puts jq text of several lines back on one (Q-026, DESIGN §23): Layout's way back, so
// F8 can go both ways. A run of space with a line break in it becomes one space, or nothing
// just inside a bracket, so a laid-out bracket closes up again; a comment runs to its line's
// end, so the break after one stays. Space without a break is left as it was written, and the
// tokens are the text's own, so the meaning is.
func Join(text string) string {
	ts := Lex(text)
	var b strings.Builder
	for i, t := range ts {
		switch {
		case t.Kind == Space && (i == 0 || i == len(ts)-1): // around the text: gone
		case t.Kind != Space || !strings.Contains(t.Text, "\n"):
			b.WriteString(t.Text)
		case ts[i-1].Kind == Comment:
			b.WriteString(lineBreak(ts[i-1].Text))
		case opener(ts[i-1]) || closer(ts[i+1]):
		default:
			b.WriteString(" ")
		}
	}
	return b.String()
}

func opener(t Token) bool {
	return t.Kind == Bracket && strings.Contains("([{", t.Text) || t.Kind == Interp && t.Text == `\(`
}

func closer(t Token) bool {
	return (t.Kind == Bracket || t.Kind == Interp) && strings.Contains(")]}", t.Text)
}

// lineBreak is the break to end a line with: \n, but \r after a comment that a \ carried over
// a lone \r, where a \n would make the pair \r\n and carry the comment on over the next line.
func lineBreak(line string) string {
	if strings.HasSuffix(line, "\r") {
		return "\r"
	}
	return "\n"
}
