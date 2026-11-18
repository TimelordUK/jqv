package jqsyntax

import (
	"strings"
	"unicode/utf8"
)

// indent is how far an opened group's inside is set in from the line its opener is on.
const indent = 4

// Width is the width Layout fills when the caller knows of none.
const Width = 80

// Layout lays jq text out over lines by its structure (Q-035, DESIGN §19), to read: one stage
// a line, each top-level | starting one, and a group (a bracket, an if, a def) opened up only
// when it doesn't fit in width from where it starts (DESIGN §23, owner 2026-11-18: a whole
// program laid out by "open every bracket with a pipeline in it" ran to twice its lines). So
//
//	[.[].people[] | select(.age > 30)] | reduce .[].age as $i (0; . + $i)
//
// is
//
//	[.[].people[] | select(.age > 30)]
//	| reduce .[].age as $i (0; . + $i)
//
// and a group too wide is opened, outermost first: its inside indented, its closer back on a
// line of its own, each | inside starting a line, and each comma inside [ ] or { } (each ;
// inside ( )) ending one. A def opens after its colon. An if opens with an elif or when longer
// than ifWidth (owner 2026-11-10, from use), and so do the brackets around it: the condition
// on the if's line, each branch indented under then, elif and else. A comment ends its line
// and opens the groups around it; a comment written after code stays after it, one on a line
// of its own keeps a line of its own, and so does a blank line between lines. A string's \(…)
// is never opened. Spaces are kept as one; the tokens, and so the meaning, are the text's own.
// Half-typed text lays out as far as it goes.
func Layout(text string, width int) string {
	ts := Lex(text)
	gs := groups(ts)
	l := &lines{}
	levels := []level{{kind: 't', open: true}}
	top := func() *level { return &levels[len(levels)-1] }
	// opens says whether the group starting at ts[i] opens: it must, or it doesn't fit from
	// where it starts
	opens := func(i int) bool {
		g := gs[i]
		return g.must || l.col()+g.width+g.tail > width
	}
	// closeBracket ends the innermost bracket (or \() and what is left open inside it
	closeBracket := func() (level, bool) {
		for i := len(levels) - 1; i > 0; i-- {
			if k := levels[i].kind; k != 'f' && k != 'd' && k != 'o' {
				lv := levels[i]
				levels = levels[:i]
				return lv, true
			}
		}
		return level{}, false
	}
	push := func(k rune, open bool) level {
		next := level{kind: k, open: open, indent: top().indent, head: k == 'd'}
		if open {
			next.indent += indent
		}
		levels = append(levels, next)
		return next
	}
	for i, t := range ts {
		if top().kind == 'o' && endsOperand(t) { // what ends a line broken at an operator
			levels = levels[:len(levels)-1]
		}
		lv := top()
		switch {
		case t.Kind == Space:
			// a line break just inside a bracket is nothing when the bracket stays shut, as in Join
			l.space = !strings.Contains(t.Text, "\n") || i == 0 || i == len(ts)-1 || !opener(ts[i-1]) && !closer(ts[i+1])
			if strings.Count(t.Text, "\n") > 1 {
				l.blank = true
			}
		case t.Kind == Comment:
			after := i > 0 && ts[i-1].Kind != Comment && !(ts[i-1].Kind == Space && strings.ContainsAny(ts[i-1].Text, "\r\n"))
			at := lv.indent
			switch {
			case after && l.pending: // written after code: it stays there, and the break follows it
				at = l.pendingAt
				l.pending, l.space = false, true
			case !after:
				l.newline(lv.indent)
			}
			l.write(t.Text)
			l.brk = lineBreak(t.Text)
			l.newline(at)
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Interp && t.Text == `\(`:
			k, open := rune(t.Text[0]), false
			if t.Kind == Interp {
				k = 'i'
			} else if !gs[i].hug {
				open = opens(i)
			}
			l.write(t.Text)
			if next := push(k, open); open {
				l.newline(next.indent)
			}
		case t.Kind == Bracket, t.Kind == Interp:
			if closed, ok := closeBracket(); ok && closed.open {
				l.newline(top().indent)
			}
			l.write(t.Text)
		case t.Kind == Keyword && (t.Text == "if" || t.Text == "def"):
			open := opens(i)
			l.write(t.Text)
			push(map[string]rune{"if": 'f', "def": 'd'}[t.Text], open)
		case t.Kind == Punct && t.Text == ":" && lv.kind == 'd' && lv.head:
			lv.head = false
			l.write(t.Text)
			if lv.open {
				l.newline(lv.indent)
			}
		case t.Kind == Punct && t.Text == ";" && lv.kind == 'd':
			l.write(t.Text)
			levels = levels[:len(levels)-1]
			if top().open {
				l.newline(top().indent)
			}
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
			if lv.kind == 'f' {
				levels = levels[:len(levels)-1]
				if lv.open {
					l.newline(top().indent)
				}
			}
			l.write(t.Text)
		case binary(t) && (lv.open || lv.kind == 'o') && l.col()+1+operand(ts, i) > width:
			// too long for the line: it breaks before the operator, a step in from its stage
			if lv.kind != 'o' {
				levels = append(levels, level{kind: 'o', indent: lv.indent + indent})
			}
			l.newline(top().indent)
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

// level is a group being written: its kind ('(' '[' '{' a bracket, 'f' an if, 'd' a def, 'i'
// a string's \(, 'o' the lines after an operator a line broke before, 't' the top), whether it is opened up over lines, the indent of the lines
// inside it, and for a def whether its colon is still to come.
type level struct {
	kind   rune
	open   bool
	indent int
	head   bool
}

// ifWidth is how long an if can be, from if to end with spaces counted as one, and stay on its
// line. The same 60 as compactwidth (DESIGN §14).
const ifWidth = 60

// group is what Layout needs to know of a group before writing it: how wide it is on one line,
// how wide what follows it is up to where the line can break (`) as $x`), and whether it must
// open whatever the width.
type group struct {
	width, tail int
	must        bool
	hug         bool // a ( or [ holding one group and nothing else: it stays shut around it, map({…})
}

// groups are the groups by the index of their first token: brackets, ifs, defs and strings'
// \(. A group must open when it holds a comment; an if must with an elif or when longer than
// ifWidth, and then so must the brackets and ifs around it.
func groups(ts []Token) map[int]*group {
	gs := map[int]*group{}
	var stack []int
	ends := map[int]int{}
	elif := map[int]bool{}
	isIf := func(o int) bool { return ts[o].Kind == Keyword && ts[o].Text == "if" }
	isDef := func(o int) bool { return ts[o].Kind == Keyword && ts[o].Text == "def" }
	mustAll := func(defsToo bool) {
		for _, o := range stack {
			if defsToo || !isDef(o) {
				gs[o].must = true
			}
		}
	}
	for i, t := range ts {
		n := len(stack)
		switch {
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Interp && t.Text == `\(`,
			t.Kind == Keyword && (t.Text == "if" || t.Text == "def"):
			gs[i] = &group{}
			stack = append(stack, i)
		case t.Kind == Bracket, t.Kind == Interp:
			for len(stack) > 0 { // the bracket, and the ifs and defs left open inside it
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				ends[o] = i
				if ts[o].Kind != Keyword {
					break
				}
			}
		case t.Kind == Keyword && t.Text == "elif" && n > 0 && isIf(stack[n-1]):
			elif[stack[n-1]] = true
		case t.Kind == Keyword && t.Text == "end" && n > 0 && isIf(stack[n-1]):
			f := stack[n-1]
			ends[f] = i
			stack = stack[:n-1]
			if elif[f] || width(ts[f:i+1]) > ifWidth {
				gs[f].must = true
				mustAll(false)
			}
		case t.Kind == Punct && t.Text == ";" && n > 0 && isDef(stack[n-1]):
			ends[stack[n-1]] = i
			stack = stack[:n-1]
		case t.Kind == Comment:
			mustAll(true)
		}
	}
	for o, g := range gs {
		end, ok := ends[o]
		if !ok {
			end = len(ts) - 1
		}
		g.width = width(ts[o : end+1])
		if !isDef(o) {
			g.tail = tail(ts, end+1)
		}
		if ok && (ts[o].Text == "(" || ts[o].Text == "[") {
			first, last := o+1, end-1
			for first < end && ts[first].Kind == Space {
				first++
			}
			for last > o && ts[last].Kind == Space {
				last--
			}
			if inner, ok := ends[first]; ok && inner == last && ts[first].Kind == Bracket {
				g.hug = true
			}
		}
	}
	return gs
}

// tail is how wide what follows a group from ts[i] is, up to where its line can break: a | ,
// or ; at the group's own depth (a , or ; stays on the line, so it counts), the next group's
// opener (it can open), a comment, an if's keyword, or the closer of the bracket around it.
func tail(ts []Token, i int) int {
	start, depth := i, 0
	for ; i < len(ts); i++ {
		t := ts[i]
		switch {
		case t.Kind == Comment:
			return width(ts[start:i])
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Keyword && t.Text == "if":
			return width(ts[start : i+1])
		case t.Kind == Interp && t.Text == `\(`:
			depth++
		case t.Kind == Bracket, t.Kind == Interp:
			if depth == 0 {
				return width(ts[start:i])
			}
			depth--
		case depth == 0 && t.Kind == Pipe:
			return width(ts[start:i])
		case depth == 0 && t.Kind == Punct && t.Text != ":":
			return width(ts[start : i+1])
		case depth == 0 && t.Kind == Keyword && (t.Text == "then" || t.Text == "elif" || t.Text == "else" || t.Text == "end"):
			return width(ts[start:i])
		}
	}
	return width(ts[start:])
}

// binary says whether t is an operator a long line may break before: arithmetic, //, and, or.
func binary(t Token) bool {
	switch {
	case t.Kind == Operator:
		return strings.Contains(" + - * / % // ", " "+t.Text+" ")
	case t.Kind == Keyword:
		return t.Text == "and" || t.Text == "or"
	}
	return false
}

// endsOperand says whether t ends the operand after a binary operator at its depth: a stage,
// an item, a branch or the bracket around it ends.
func endsOperand(t Token) bool {
	return t.Kind == Pipe || t.Kind == Punct && t.Text != ":" ||
		(t.Kind == Bracket || t.Kind == Interp) && strings.Contains(")]}", t.Text) ||
		t.Kind == Keyword && (t.Text == "then" || t.Text == "elif" || t.Text == "else" || t.Text == "end")
}

// operand is how wide the binary operator at ts[i] and its operand are on one line, up to the
// next operator, or what ends the operand, at its depth.
func operand(ts []Token, i int) int {
	start, depth := i, 0
	for i++; i < len(ts); i++ {
		t := ts[i]
		switch {
		case t.Kind == Comment:
			return width(ts[start:i])
		case t.Kind == Bracket && strings.Contains("([{", t.Text), t.Kind == Interp && t.Text == `\(`,
			t.Kind == Keyword && t.Text == "if":
			depth++
		case depth > 0 && ((t.Kind == Bracket || t.Kind == Interp) && strings.Contains(")]}", t.Text) ||
			t.Kind == Keyword && t.Text == "end"):
			depth--
		case depth == 0 && (endsOperand(t) || binary(t)):
			return width(ts[start:i])
		}
	}
	return width(ts[start:])
}

// width is how long tokens are on one line, measured so that laying them out doesn't change
// it, or laying out again could decide otherwise: the space next to what Layout places (a
// bracket's inside, | , ; :, an operator it may break before, an if's keywords, a comment) is
// Layout's, so it is not counted from the text; those tokens count as written with Layout's
// spaces (` | `, `, `, ` + `, ` then `). Other space counts as one.
func width(ts []Token) int {
	n := 0
	for k, t := range ts {
		switch {
		case t.Kind != Space:
			n += utf8.RuneCountInString(t.Text)
			switch {
			case t.Kind == Pipe, binary(t), t.Kind == Keyword && (t.Text == "then" || t.Text == "elif" || t.Text == "else" || t.Text == "end"):
				n += 2
			case t.Kind == Punct:
				n++
			}
		case k > 0 && k < len(ts)-1 && !placed(ts[k-1], true) && !placed(ts[k+1], false):
			n++
		}
	}
	return n
}

// placed says whether the space after t (or before it, when after is false) is Layout's to
// place rather than the text's.
func placed(t Token, after bool) bool {
	switch {
	case t.Kind == Pipe, t.Kind == Punct, t.Kind == Comment, binary(t),
		t.Kind == Keyword && (t.Text == "then" || t.Text == "elif" || t.Text == "else" || t.Text == "end"):
		return true
	case after:
		return t.Kind == Bracket && strings.Contains("([{", t.Text)
	}
	return (t.Kind == Bracket || t.Kind == Interp) && strings.Contains(")]}", t.Text)
}

// lines builds the laid-out text: a space is written only between two tokens on a line, and a
// line is ended only when something is written after it, so a comment written after code can
// still go on the line before the break.
type lines struct {
	b         strings.Builder
	line      strings.Builder
	at        int // the indent the current line started with
	space     bool
	brk       string // the break ending this line, when a comment wants \r (lineBreak); "" is \n
	pending   bool   // the line ends before what is written next
	pendingAt int    // the next line's indent
	blank     bool   // a blank line was written here: keep one before the next line
}

// col is the column the next token starts at.
func (l *lines) col() int {
	if l.pending {
		return l.pendingAt
	}
	n := utf8.RuneCountInString(l.line.String())
	if l.space && n > l.at {
		n++
	}
	return n
}

func (l *lines) write(s string) {
	if l.pending {
		brk := l.brk
		if brk == "" {
			brk = "\n"
		}
		l.b.WriteString(l.line.String() + brk) // no trailing space is ours: a string's own stay
		l.brk, l.pending = "", false
		l.start(l.pendingAt)
	}
	if l.blank && l.line.Len() == l.at && l.b.Len() > 0 {
		l.b.WriteString("\n")
	}
	l.blank = false
	if l.space && l.line.Len() > l.at {
		l.line.WriteByte(' ')
	}
	l.space = false
	l.line.WriteString(s)
}

// newline ends the line, if it has anything on it, before what is written next.
func (l *lines) newline(indent int) {
	l.space = false
	if l.pending || l.line.Len() > l.at {
		l.pending, l.pendingAt = true, indent
		return
	}
	l.start(indent)
}

func (l *lines) start(indent int) {
	l.line.Reset()
	l.line.WriteString(strings.Repeat(" ", indent))
	l.at = indent
}

func (l *lines) String() string {
	if l.line.Len() > l.at {
		l.b.WriteString(l.line.String())
	}
	return l.b.String()
}
