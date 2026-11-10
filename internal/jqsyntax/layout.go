package jqsyntax

import (
	"strings"
	"unicode/utf8"
)

// indent is how far a bracket's inside is set in from the line its opener is on.
const indent = 4

// Layout lays jq text out over lines by its structure (Q-035, DESIGN §19), to read, not to
// keep: one stage a line, each | starting one, and a bracket with a pipeline anywhere inside it
// opened up, its inside indented and its closer back on a line of its own. So
//
//	[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)
//
// is
//
//	[
//	    .[].people[]
//	    | select(.age > 30)
//	]
//	| reduce .[].age as $i (0; . + $i)
//
// A bracket with no pipeline in it stays on its line, as written. Inside an opened [ ] or { }
// each comma ends a line; inside an opened ( ) each ;. An if with a pipeline in it, an elif,
// or longer than ifWidth is opened too, with the brackets around it: the condition on the if's
// line, then each branch indented under then, elif and else.
// A string's \(…) is never opened. A ; outside any bracket ends a def, so it ends a line, and a
// comment ends its line. Spaces are kept as one space; the tokens, and so the meaning, are the
// text's own. Half-typed text lays out as far as it goes.
func Layout(text string) string {
	ts := Lex(text)
	open := opened(ts)
	l := &lines{}
	levels := []level{{kind: 't', open: open[-1]}}
	top := func() *level { return &levels[len(levels)-1] }
	// pop closes the innermost level a closer of kind k ends: a bracket's closer ends the
	// brackets and ifs left open inside it; end ends an if only.
	pop := func(k rune) (level, bool) {
		for i := len(levels) - 1; i > 0; i-- {
			if k == 'f' && levels[i].kind != 'f' {
				return level{}, false
			}
			if k == 'f' || levels[i].kind != 'f' {
				lv := levels[i]
				levels = levels[:i]
				return lv, true
			}
		}
		return level{}, false
	}
	for i, t := range ts {
		lv := top()
		switch {
		case t.Kind == Space:
			l.space = true
		case t.Kind == Comment:
			l.write(t.Text)
			l.newline(lv.indent)
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Interp && t.Text == `\(`:
			l.write(t.Text)
			k := rune(t.Text[0])
			if t.Kind == Interp {
				k = 'i'
			}
			next := level{kind: k, open: open[i], indent: lv.indent}
			if next.open {
				next.indent += indent
			}
			levels = append(levels, next)
			if next.open {
				l.newline(next.indent)
			}
		case t.Kind == Bracket, t.Kind == Interp:
			if closed, ok := pop(0); ok && closed.open {
				l.newline(top().indent)
			}
			l.write(t.Text)
		case t.Kind == Keyword && t.Text == "if":
			l.write(t.Text)
			next := level{kind: 'f', open: open[i], indent: lv.indent}
			if next.open {
				next.indent += indent
			}
			levels = append(levels, next)
		case t.Kind == Keyword && lv.kind == 'f' && lv.open && t.Text == "then":
			l.write(t.Text)
			l.newline(lv.indent)
		case t.Kind == Keyword && lv.kind == 'f' && lv.open && (t.Text == "elif" || t.Text == "else"):
			l.newline(lv.indent - indent)
			l.write(t.Text)
			if t.Text == "else" {
				l.newline(lv.indent)
			}
		case t.Kind == Keyword && t.Text == "end":
			if closed, ok := pop('f'); ok && closed.open {
				l.newline(top().indent)
			}
			l.write(t.Text)
		case t.Kind == Pipe && lv.open:
			l.newline(lv.indent)
			l.write(t.Text)
			l.space = true
		case t.Kind == Punct && t.Text == "," && lv.open && (lv.kind == '[' || lv.kind == '{'),
			t.Kind == Punct && t.Text == ";" && (lv.kind == 't' || lv.open && lv.kind == '('):
			l.write(t.Text)
			l.newline(lv.indent)
		default:
			l.write(t.Text)
		}
	}
	return l.String()
}

// level is a bracket, an if, a string's \(, or the top: its kind ('(' '[' '{' 'f' 'i' 't'),
// whether it is opened up over lines, and the indent of the lines inside it.
type level struct {
	kind   rune
	open   bool
	indent int
}

// ifWidth is how long an if can be, from if to end with spaces counted as one, and stay on its
// line. The same 60 as compactwidth (DESIGN §14).
const ifWidth = 60

// opened says, by the index of each opener (bracket, if, a string's \(), whether it is opened
// up: it has a pipeline in it, at any depth outside a string's \(…), or holds an opened if.
// An if opens when it has a pipeline, an elif, or is longer than ifWidth. -1 is the top.
func opened(ts []Token) map[int]bool {
	open := map[int]bool{}
	var stack []int // the openers' indexes
	interp := 0     // how many of them are a string's \(
	elif := map[int]bool{}
	openAll := func() { // the stack and the top
		open[-1] = true
		for _, o := range stack {
			open[o] = true
		}
	}
	pop := func(onlyIf bool) {
		for len(stack) > 0 {
			o := ts[stack[len(stack)-1]]
			isIf := o.Kind == Keyword
			if onlyIf && !isIf {
				return
			}
			stack = stack[:len(stack)-1]
			if o.Kind == Interp {
				interp--
			}
			if onlyIf || !isIf {
				return
			}
		}
	}
	for i, t := range ts {
		switch {
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Keyword && t.Text == "if":
			stack = append(stack, i)
		case t.Kind == Interp && t.Text == `\(`:
			stack = append(stack, i)
			interp++
		case t.Kind == Bracket, t.Kind == Interp:
			pop(false)
		case t.Kind == Keyword && t.Text == "elif" && len(stack) > 0 && ts[stack[len(stack)-1]].Kind == Keyword:
			elif[stack[len(stack)-1]] = true
		case t.Kind == Keyword && t.Text == "end":
			if n := len(stack); n > 0 && ts[stack[n-1]].Kind == Keyword && interp == 0 {
				if f := stack[n-1]; elif[f] || width(ts[f:i+1]) > ifWidth {
					openAll() // the if, and the brackets around it
				}
			}
			pop(true)
		case t.Kind == Pipe && interp == 0:
			openAll()
		}
	}
	return open
}

// width is how long tokens are on one line, with each run of spaces as one.
func width(ts []Token) int {
	n := 0
	for _, t := range ts {
		if t.Kind == Space {
			n++
		} else {
			n += utf8.RuneCountInString(t.Text)
		}
	}
	return n
}

// lines builds the laid-out text: a space is written only between two tokens on a line, and a
// line is ended only when it has something on it.
type lines struct {
	b     strings.Builder
	line  strings.Builder
	at    int // the indent the current line started with
	space bool
}

func (l *lines) write(s string) {
	if l.space && l.line.Len() > l.at {
		l.line.WriteByte(' ')
	}
	l.space = false
	l.line.WriteString(s)
}

func (l *lines) newline(indent int) {
	l.space = false
	if l.line.Len() > l.at {
		l.b.WriteString(l.line.String() + "\n") // no trailing space is ours: a string's own stay
	}
	l.line.Reset()
	l.line.WriteString(strings.Repeat(" ", indent))
	l.at = indent
}

func (l *lines) String() string {
	l.newline(0)
	return strings.TrimSuffix(l.b.String(), "\n")
}
