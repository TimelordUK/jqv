package jqsyntax

import (
	"fmt"
	"slices"
	"strings"
)

// Open is something jq text leaves unfinished (Q-026, DESIGN §23): a bracket, a string, an
// if or a def not yet closed, or a token at the end that wants more after it. Enter goes on to
// the next line on an entry with one, and a line that runs anyway (unexpected EOF) is told
// which.
type Open struct {
	Pos, End int    // the opener, or the token at the end, in runes
	Text     string // as shown: select(, {, ", \(, if, def f, |, and, as $x
	Close    string // what closes it: ) ] } " end ; and "" for a token at the end
	Stray    int    // where a closer came that doesn't close it (] for select(), else -1
}

// wantsMore are the keywords that can't end a program.
var wantsMore = map[string]bool{"and": true, "or": true, "if": true, "then": true, "elif": true,
	"else": true, "as": true, "reduce": true, "foreach": true, "try": true, "catch": true,
	"label": true, "break": true, "def": true, "import": true, "include": true, "module": true}

// Unfinished is what text leaves open, outermost first, then a token at the end that wants
// more. Text gojq parses has none. Closers that close the wrong thing (`select(.a]`) or close
// a bracket around an unfinished if or def (`(if . then 1)`) mark it with Stray.
func Unfinished(text string) []Open {
	ts := Lex(text)
	var code []Token // without spaces and comments
	for _, t := range ts {
		if t.Kind != Space && t.Kind != Comment {
			code = append(code, t)
		}
	}
	type frame struct {
		Open
		depth int  // the depth its ; or its body's ( is at
		as    bool // a reduce or foreach past its as: the next ) at its depth ends it
	}
	var stack []frame
	var done []Open
	push := func(t Token, show, close string) {
		stack = append(stack, frame{Open{Pos: t.Pos, End: t.End, Text: show, Close: close, Stray: -1}, t.Depth, false})
	}
	top := func() *frame {
		if len(stack) == 0 {
			return nil
		}
		return &stack[len(stack)-1]
	}
	for i, t := range code {
		switch {
		case t.Kind == Quote:
			if o := top(); o != nil && o.Close == `"` {
				stack = stack[:len(stack)-1]
			} else {
				push(t, `"`, `"`)
			}
		case t.Kind == Interp && t.Text == `\(`:
			push(t, `\(`, ")")
		case t.Kind == Interp: // the ) that ends a \(
			stack = stack[:len(stack)-1]
		case t.Kind == Bracket && strings.Contains("([{", t.Text):
			show, at := t.Text, t
			if t.Text == "(" && i > 0 && code[i-1].Kind == Name && code[i-1].End == t.Pos {
				show, at.Pos = code[i-1].Text+"(", code[i-1].Pos // a call: shown from its name
			}
			push(at, show, map[string]string{"(": ")", "[": "]", "{": "}"}[t.Text])
		case t.Kind == Bracket: // a closer the lexer matched: what is open inside it never closed
			for len(stack) > 0 && !isBracket(top().Close) {
				o := top().Open
				o.Stray = t.Pos
				done = append(done, o)
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if o := top(); o != nil && o.as && t.Text == ")" && t.Depth == o.depth {
				stack = stack[:len(stack)-1] // reduce … as $x (…): its body closed
			}
		case t.Kind == Error && len(t.Text) == 1 && strings.Contains(")]}", t.Text):
			for j := len(stack) - 1; j >= 0; j-- { // the bracket it was meant for
				if isBracket(stack[j].Close) && stack[j].Stray < 0 {
					stack[j].Stray = t.Pos
					break
				}
			}
		case t.Kind == Keyword && t.Text == "if":
			push(t, "if", "end")
		case t.Kind == Keyword && t.Text == "end":
			if o := top(); o != nil && o.Close == "end" {
				stack = stack[:len(stack)-1]
			}
		case t.Kind == Keyword && (t.Text == "reduce" || t.Text == "foreach"):
			push(t, t.Text, "(…)")
		case t.Kind == Keyword && t.Text == "as":
			if o := top(); o != nil && o.Close == "(…)" && t.Depth == o.depth {
				o.as = true
			}
		case t.Kind == Keyword && (t.Text == "def" || t.Text == "import" || t.Text == "include"):
			show := t.Text
			if i+1 < len(code) && code[i+1].Kind == Name {
				show += " " + code[i+1].Text
			}
			push(t, show, ";")
		case t.Kind == Punct && t.Text == ";":
			if o := top(); o != nil && o.Close == ";" && t.Depth == o.depth {
				stack = stack[:len(stack)-1]
			}
		}
	}
	out := done
	for i, f := range stack {
		if !f.as || i == len(stack)-1 { // a reduce past its as: the ( of its body, open after it, says it
			out = append(out, f.Open)
		}
	}
	slices.SortStableFunc(out, func(a, b Open) int { return a.Pos - b.Pos })
	if len(code) > 0 {
		last := code[len(code)-1]
		wants := false
		show := last.Text
		inReduce := len(stack) > 0 && stack[len(stack)-1].Close == "(…)"
		if p := pattern(code); p >= 0 && !inReduce { // X as $x, X as [$a, $b]: the body comes after a |
			wants, show, last = true, joined(code[p:]), code[p]
		}
		switch last.Kind {
		case Pipe:
			wants = true
		case Punct:
			wants = last.Text != ";"
		case Operator:
			wants = last.Text != "?"
		case Keyword:
			wants = wantsMore[last.Text]
		case Dot: // A. and .a . : after a term, a . is an index that wants its key
			wants = len(code) > 1 && endsTerm(code[len(code)-2])
		}
		if wants && (len(out) == 0 || out[len(out)-1].Pos != last.Pos) {
			out = append(out, Open{Pos: last.Pos, End: last.End, Text: show, Stray: -1})
		}
	}
	return out
}

// pattern is where an `as $x`, `as [$a, $b]` or `label $out` that ends the code starts, or -1.
func pattern(code []Token) int {
	j := len(code) - 1
	switch t := code[j]; {
	case t.Kind == Variable:
		j--
	case t.Kind == Bracket && (t.Text == "]" || t.Text == "}"):
		for j--; j >= 0 && !(code[j].Kind == Bracket && code[j].Depth == t.Depth); j-- {
		}
		j--
	default:
		return -1
	}
	if j >= 0 && code[j].Kind == Keyword && (code[j].Text == "as" || code[j].Text == "label") {
		return j
	}
	return -1
}

// joined is tokens as written, with one space after `as`/`label` and after commas.
func joined(ts []Token) string {
	var b strings.Builder
	for i, t := range ts {
		b.WriteString(t.Text)
		if i == 0 || t.Text == "," {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// endsTerm is whether t can be the end of a term, so a . after it is an index.
func endsTerm(t Token) bool {
	switch t.Kind {
	case Name, Field, Variable, Dot, Recurse, Number, Quote, Format:
		return true
	case Bracket:
		return strings.Contains(")]}", t.Text)
	case Keyword:
		return t.Text == "end" || t.Text == "null" || t.Text == "true" || t.Text == "false"
	case Operator:
		return t.Text == "?"
	}
	return false
}

func isBracket(close string) bool { return close == ")" || close == "]" || close == "}" }

// Explain says in a sentence what an entry leaves open and how to finish it, for a line that
// failed to parse: `select( at column 7 is still open: close it with )`. "" when nothing is.
func Explain(text string, os []Open) string {
	var parts, open []string
	var closers []string
	for _, o := range os {
		at := where(text, o.Pos)
		switch {
		case o.Close == "":
			parts = append(parts, fmt.Sprintf("%s at the end %s", o.Text, wanted(o.Text)))
		case o.Stray >= 0 && isBracket(o.Close):
			parts = append(parts, fmt.Sprintf("%s at %s is closed by %s at %s: write %s",
				o.Text, at, string([]rune(text)[o.Stray]), where(text, o.Stray), o.Close))
		case o.Stray >= 0:
			parts = append(parts, fmt.Sprintf("%s at %s has no %s before the %s at %s",
				o.Text, at, o.Close, string([]rune(text)[o.Stray]), where(text, o.Stray)))
		case o.Close == ";" && strings.HasPrefix(o.Text, "def"):
			parts = append(parts, fmt.Sprintf("%s at %s wants a ; after its body", o.Text, at))
		case o.Close == ";":
			parts = append(parts, fmt.Sprintf("%s at %s wants a ; to end it", o.Text, at))
		case o.Close == "(…)":
			parts = append(parts, fmt.Sprintf("%s at %s is not finished: %s SOURCE as $x (INIT; UPDATE)", o.Text, at, o.Text))
		default:
			open = append(open, fmt.Sprintf("%s at %s", o.Text, at))
			closers = append([]string{o.Close}, closers...)
		}
	}
	switch len(open) {
	case 0:
	case 1:
		parts = append(parts, fmt.Sprintf("%s is still open: close it with %s", open[0], closers[0]))
	default:
		parts = append(parts, fmt.Sprintf("still open: %s: close them with %s",
			strings.Join(open, ", "), strings.Join(closers, " ")))
	}
	return strings.Join(parts, "; ")
}

// wanted is what a token at the end of a program is waiting for.
func wanted(tok string) string {
	switch {
	case tok == "|":
		return "wants a next stage"
	case tok == ".":
		return "wants a key after it"
	case tok == ",":
		return "wants another output after it"
	case strings.HasPrefix(tok, "as "):
		return "wants | and what uses " + strings.TrimPrefix(tok, "as ")
	case strings.HasPrefix(tok, "label "):
		return "wants | and what breaks out of it"
	}
	return "wants something after it"
}

// where is a position as `column 7`, or `line 2 column 3` in text of several lines.
func where(text string, pos int) string {
	rs := []rune(text)
	line, col := 1, 1
	for _, r := range rs[:min(pos, len(rs))] {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	if !slices.Contains(rs, '\n') {
		return fmt.Sprintf("column %d", col)
	}
	return fmt.Sprintf("line %d column %d", line, col)
}
