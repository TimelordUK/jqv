package assist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/scope"
	"github.com/TimelordUK/jqv/internal/shape"
)

// comparisons are the operators whose right side Tab fills with the left side's values.
var comparisons = map[string]bool{"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true}

// valueCalls are the functions whose arguments are compared with their input, so Tab fills
// them with the input's values: `.status | IN("Pending", "C▮`, `.name | startswith("A▮`.
var valueCalls = map[string]bool{"IN": true, "startswith": true, "endswith": true, "test": true,
	"contains": true, "ltrimstr": true, "rtrimstr": true, "inside": true, "index": true}

// values are what a value at the cursor can be (R-006, DESIGN §22 step 6): after a
// comparison, the values seen where its left side points (`select(.status == ▮` cycles
// "Completed", "In Progress", "Pending"); in a string argument of IN, startswith and the
// like, the values of their input. Most frequent first; what is typed inside the string (or
// the number) keeps those that start with it, then those containing it in any case. Each
// goes in as JSON text, quotes and escapes and all, so it can't break the line. ok is false
// where no value is wanted here, for the next completer to try.
func values(s *bench.Session, line string, cur int) (from, to int, cs []editor.Candidate, ok bool) {
	off, expr, isJQ := jqPart(line)
	if !isJQ || cur < off {
		return 0, 0, nil, false
	}
	c := cur - off
	toks := jqsyntax.Lex(expr)
	in := -1 // the token the cursor is in or just after
	for i, t := range toks {
		if t.Pos < c && c <= t.End {
			in = i
		}
	}
	// what is being typed, where it goes, and the token before it
	var typed string
	before := -1
	from, to = c, c
	space := "" // a value straight after the operator gets a space before it
	switch {
	case in >= 0 && (toks[in].Kind == jqsyntax.Text || toks[in].Kind == jqsyntax.Escape || toks[in].Kind == jqsyntax.Quote && opening(toks, in)):
		q := in // the string's opening quote: the one at the cursor, or the last before it
		if toks[q].Kind != jqsyntax.Quote {
			for q--; q >= 0 && toks[q].Kind != jqsyntax.Quote; q-- {
			}
			if q < 0 || !opening(toks, q) {
				return 0, 0, nil, false
			}
		}
		typed = string([]rune(expr)[toks[q].End:c])
		from, to = toks[q].Pos, c
		for i := q + 1; i < len(toks); i++ {
			if k := toks[i].Kind; k != jqsyntax.Text && k != jqsyntax.Escape && k != jqsyntax.Quote {
				break
			}
			to = toks[i].End
			if toks[i].Kind == jqsyntax.Quote {
				break
			}
		}
		before = prevCode(toks, q)
	case in >= 0 && toks[in].Kind == jqsyntax.Number:
		typed = string([]rune(expr)[toks[in].Pos:c])
		from, to = toks[in].Pos, toks[in].End
		before = prevCode(toks, in)
	case in >= 0 && (toks[in].Kind == jqsyntax.Space || toks[in].Kind == jqsyntax.Operator || toks[in].Text == "(" || toks[in].Text == ","):
		before = in
		if toks[in].Kind == jqsyntax.Space {
			before = prevCode(toks, in)
		} else if toks[in].Kind == jqsyntax.Operator {
			space = " "
		}
	default:
		return 0, 0, nil, false
	}
	if before < 0 {
		return 0, 0, nil, false
	}
	program, known := "", false
	switch b := toks[before]; {
	case b.Kind == jqsyntax.Operator && comparisons[b.Text]:
		left := prevCode(toks, before)
		if left < 0 {
			return 0, 0, nil, false
		}
		program, known = operand(expr, toks, left)
	case b.Text == "(" || b.Text == "," || b.Text == ";":
		if !valueCalls[callName(toks, before)] {
			return 0, 0, nil, false
		}
		program, known = scope.At(expr, c).Input()
	default:
		return 0, 0, nil, false
	}
	if !known {
		return cur, cur, nil, true
	}
	f := s.Shapes().Of(program)
	if f.Shape == nil || scalars(f.Shape) == 0 {
		return 0, 0, nil, false
	}
	return off + from, off + to, valueCandidates(f, typed, space), true
}

// operand is the program for a comparison's left side, whose last token is toks[last]: the
// term's own program when the side is one path (`.status`, `$e.position`), else the side's
// text back to what binds less tightly than a comparison (a pipe, a comma, and/or, a keyword,
// the bracket it is in), run on the input there: `.name | length` gives `… | length`.
func operand(expr string, toks []jqsyntax.Token, last int) (string, bool) {
	end := toks[last].End
	sc := scope.At(expr, end)
	first := last
	for i := last; i >= 0; i-- {
		t := toks[i]
		if t.Depth < toks[last].Depth || t.Depth == toks[last].Depth && boundary(t) {
			break
		}
		if t.Kind != jqsyntax.Space && t.Kind != jqsyntax.Comment {
			first = i
		}
	}
	if sc.Term.Pos == toks[first].Pos && (sc.Term.Head != "" || sc.Term.Path != "" || sc.Term.Typing) {
		return sc.Here()
	}
	in, ok := scope.At(expr, toks[first].Pos).Input()
	if !ok {
		return "", false
	}
	side := strings.TrimSpace(string([]rune(expr)[toks[first].Pos:end]))
	if in == "." {
		return side, true
	}
	return in + " | " + side, true
}

// boundary is whether t ends a comparison's operand on its left.
func boundary(t jqsyntax.Token) bool {
	switch t.Kind {
	case jqsyntax.Pipe:
		return true
	case jqsyntax.Punct:
		return t.Text == "," || t.Text == ";" || t.Text == ":"
	case jqsyntax.Keyword:
		switch t.Text {
		case "and", "or", "if", "then", "elif", "else", "as":
			return true
		}
	case jqsyntax.Operator:
		return comparisons[t.Text] || t.Text == "//" || strings.HasSuffix(t.Text, "=") && !comparisons[t.Text]
	}
	return false
}

// valueCandidates are the scalar values seen, most frequent first, matching typed.
func valueCandidates(f scope.Found, typed, space string) []editor.Candidate {
	sh := f.Shape
	vs := sh.Values()
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].N > vs[j].N })
	n, more := sh.Distinct()
	total := scalars(sh)
	low := strings.ToLower(typed)
	var prefix, inside []shape.ValueCount
	for _, v := range vs {
		text := plain(v.Text)
		switch {
		case strings.HasPrefix(text, typed):
			prefix = append(prefix, v)
		case strings.Contains(strings.ToLower(text), low):
			inside = append(inside, v)
		}
	}
	var cs []editor.Candidate
	for _, v := range append(prefix, inside...) {
		ins := jqText(v.Text)
		hint := fmt.Sprintf("%s  ×%d of %d", ins, v.N, total)
		if sh.Kinds[doc.Number] > 1 && sh.MinNum != sh.MaxNum {
			hint += "  (numbers " + strconv.FormatFloat(sh.MinNum, 'g', -1, 64) + ".." + strconv.FormatFloat(sh.MaxNum, 'g', -1, 64) + ")"
		}
		switch {
		case more:
			hint += fmt.Sprintf("  (the first %d distinct values; there are more)", n)
		case f.Cut:
			hint += "  (of the first outputs)"
		}
		cs = append(cs, editor.Candidate{Insert: space + ins, Show: ins, Hint: hint})
	}
	return cs
}

// scalars is how many scalar values were seen at a place.
func scalars(sh *shape.Shape) int {
	return sh.Kinds[doc.String] + sh.Kinds[doc.Number] + sh.Kinds[doc.Bool] + sh.Kinds[doc.Null]
}

// plain is a value's text as typed inside quotes: a string without its quotes.
func plain(text string) string {
	if s, err := strconv.Unquote(text); err == nil {
		return s
	}
	return text
}

// jqText is a shape's value text (strings as Go quotes them) as jq text: JSON, with <, >
// and & left as they are.
func jqText(text string) string {
	s, err := strconv.Unquote(text)
	if err != nil {
		return text // a number, a bool, null
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if e.Encode(s) != nil {
		return text
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// opening is whether the quote at toks[q] opens a string rather than closing one.
func opening(toks []jqsyntax.Token, q int) bool {
	n := 0
	for i := 0; i <= q; i++ {
		if toks[i].Kind == jqsyntax.Quote {
			n++
		}
	}
	return n%2 == 1
}

// callName is the function whose argument list the token at toks[i] is in or opens: the
// name before the ( at its depth; "" for none.
func callName(toks []jqsyntax.Token, i int) string {
	o := i
	if toks[i].Text != "(" {
		for o = i - 1; o >= 0; o-- {
			if toks[o].Kind == jqsyntax.Bracket && toks[o].Text == "(" && toks[o].Depth == toks[i].Depth-1 {
				break
			}
		}
	}
	if o < 0 {
		return ""
	}
	if p := prevCode(toks, o); p >= 0 && toks[p].Kind == jqsyntax.Name {
		return toks[p].Text
	}
	return ""
}

// prevCode is the index of the last token before toks[i] that isn't space or a comment, or -1.
func prevCode(toks []jqsyntax.Token, i int) int {
	for i--; i >= 0; i-- {
		if k := toks[i].Kind; k != jqsyntax.Space && k != jqsyntax.Comment {
			return i
		}
	}
	return -1
}
