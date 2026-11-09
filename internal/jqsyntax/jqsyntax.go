// Package jqsyntax reads jq text for the editor's jq-aware pieces (DESIGN §18): where the
// pipes, brackets and strings are, by position, which gojq's parser doesn't give. It isn't a
// parser: it finds structure well enough to cut a pipeline at the cursor and close it off, and
// gojq has the last word on whether the result parses.
package jqsyntax

import (
	"unicode"

	"github.com/itchyny/gojq"
)

type kind int

const (
	other kind = iota
	open       // ( [ { if, and \( in a string
	close      // ) ] } end
	pipe       // |
	sep        // , ; then elif else catch: they end a part as a pipe does, but aren't stages
)

type token struct {
	kind     kind
	pos, end int    // runes
	closer   string // for open: what closes it
}

// scan gives Lex's tokens as structure: brackets, if/end and a string's \( … ) open and
// close, pipes, and the separators; everything else (names, numbers, strings, operators) is
// other, and spaces and comments are left out.
func scan(rs []rune) []token {
	var ts []token
	depth := 0
	binding := map[int]bool{} // depths where an `as` waits for its |
	add := func(k kind, t Token, closer string) { ts = append(ts, token{k, t.Pos, t.End, closer}) }
	for _, t := range Lex(string(rs)) {
		switch t.Kind {
		case Space, Comment:
		case Bracket, Interp:
			switch t.Text {
			case "(", "[", "{", `\(`:
				if t.Text == "(" { // reduce/foreach … as $x (…): no | to come
					delete(binding, depth)
				}
				add(open, t, map[string]string{"(": ")", "[": "]", "{": "}", `\(`: `)"`}[t.Text])
				depth++
			default:
				add(close, t, "")
				depth = max(depth-1, 0)
			}
		case Pipe:
			if binding[depth] { // `. as $x | …`: the body, not a new stage
				delete(binding, depth)
				add(other, t, "")
			} else {
				add(pipe, t, "")
			}
		case Punct:
			if t.Text == ":" {
				add(other, t, "")
			} else {
				add(sep, t, "")
			}
		case Keyword:
			switch t.Text {
			case "if":
				add(open, t, " end")
				depth++
			case "end":
				add(close, t, "")
				depth = max(depth-1, 0)
			case "then", "elif", "else", "catch":
				add(sep, t, "")
			case "as":
				binding[depth] = true
				add(other, t, "")
			default:
				add(other, t, "")
			}
		case Error:
			if t.Text == ")" || t.Text == "]" || t.Text == "}" { // a stray closer still closes
				add(close, t, "")
				depth = max(depth-1, 0)
			} else {
				add(other, t, "")
			}
		default:
			add(other, t, "")
		}
	}
	return ts
}

// UpTo is the expression up to the cursor, as something to run: the text before the cursor,
// on to the end of the part the cursor is in (the next |, comma or ; at its level, or the
// bracket it is inside closing), then whatever is still open closed. In
// `[.[].people[] | select(.age > 30) | .age] | add` with the cursor on select, it is
// `[.[].people[] | select(.age > 30)]`. Empty text is `.`.
func UpTo(expr string, cursor int) string {
	rs := []rune(expr)
	cursor = min(max(cursor, 0), len(rs))
	for out := 0; out <= depthAt(rs, cursor); out++ { // a cut that doesn't parse (inside reduce's source): the level out
		if p := cutAt(rs, cursor, out); parses(p) {
			return p
		}
	}
	return cutAt(rs, cursor, 0) // gojq will say what's wrong
}

// depthAt is how many brackets are open at the cursor.
func depthAt(rs []rune, cursor int) int {
	d := 0
	for _, t := range scan(rs) {
		if t.end > cursor {
			break
		}
		switch t.kind {
		case open:
			d++
		case close:
			d = max(d-1, 0)
		}
	}
	return d
}

// cutAt cuts at the end of the part the cursor is in, out levels of brackets up from it.
func cutAt(rs []rune, cursor, out int) string {
	var stack []string // closers of the brackets open
	base := -1         // the depth the part is at, once past the cursor
	cut := len(rs)
	for _, t := range scan(rs) {
		if base < 0 && t.end > cursor {
			base = max(len(stack)-out, 0)
		}
		if base >= 0 && len(stack) == base && (t.kind == pipe || t.kind == sep || t.kind == close) {
			cut = t.pos
			break
		}
		switch t.kind {
		case open:
			stack = append(stack, t.closer)
		case close:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	s := trimEnd(rs[:cut])
	for len(s) > 0 && s[len(s)-1] == '|' { // the cursor just after a pipe
		s = trimEnd(s[:len(s)-1])
	}
	o := string(s)
	for i := len(stack) - 1; i >= 0; i-- {
		o += stack[i]
	}
	if o == "" {
		return "."
	}
	return o
}

func parses(s string) bool { _, err := gojq.Parse(s); return err == nil }

// Stops are the places to step the cursor to along a pipeline: the end of each stage, before
// its | (in brackets too), the end of a bracketed pipeline before its closing bracket, and the
// end of the text. UpTo at each runs the pipeline to that stage.
func Stops(expr string) []int {
	rs := []rune(expr)
	piped := []bool{false} // per level: has it a pipe
	var stops []int
	add := func(i int) {
		i = len(trimEnd(rs[:i]))
		if len(stops) == 0 || stops[len(stops)-1] < i {
			stops = append(stops, i)
		}
	}
	for _, t := range scan(rs) {
		switch t.kind {
		case pipe:
			piped[len(piped)-1] = true
			add(t.pos)
		case open:
			piped = append(piped, false)
		case close:
			if len(piped) > 1 {
				if piped[len(piped)-1] {
					add(t.pos)
				}
				piped = piped[:len(piped)-1]
			}
		}
	}
	if len(trimEnd(rs)) > 0 {
		add(len(rs))
	}
	// keep the stops that run something new: a cut inside reduce's source runs the reduce, as
	// the stop after it does
	var keep []int
	last := ""
	for _, s := range stops {
		if p := UpTo(expr, s); p != last && parses(p) {
			keep, last = append(keep, s), p
		}
	}
	return keep
}

func trimEnd(rs []rune) []rune {
	for len(rs) > 0 && unicode.IsSpace(rs[len(rs)-1]) {
		rs = rs[:len(rs)-1]
	}
	return rs
}
