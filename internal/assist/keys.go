package assist

import (
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/scope"
	"github.com/TimelordUK/jqv/internal/shape"
)

// CompleteOn is Tab on a session (Q-036): the keys at the cursor's place when a field or an
// object's key is being typed (Q-007, DESIGN §22), else Complete's names.
func CompleteOn(s *bench.Session) editor.Completer {
	return func(line string, cur int) (int, int, []editor.Candidate) {
		if from, to, cs, ok := keys(s, line, cur); ok {
			return from, to, cs
		}
		return Complete(line, cur)
	}
}

// keys are what a field being typed can be: `.[].em▮` offers employee, `.▮` after a pipe
// every key of what comes through it, `."pa▮` the keys in quotes, and `{na▮` the input's
// keys as shorthand ({name} is {name: .name}). The key typed so far matches a key's start
// first, then anywhere in it, any case (`.id▮` finds projectId). Keys come in the order the
// document has them, and each hint is what the key holds, one level, as :scope says it.
// ok is false where no key is being typed, for Complete to try.
func keys(s *bench.Session, line string, cur int) (from, to int, cs []editor.Candidate, ok bool) {
	off, expr, isJQ := jqPart(line)
	if !isJQ || cur < off {
		return 0, 0, nil, false
	}
	c := cur - off
	sc := scope.At(expr, c)
	toks := jqsyntax.Lex(expr)
	at := -1 // the token the cursor is in or just after
	for i, t := range toks {
		if t.Pos < c && c <= t.End {
			at = i
		}
	}
	var program, typed string
	var known, quoted bool
	switch {
	case sc.Term.Typing && at >= 0:
		program, known = sc.Value()
		typed, quoted = sc.Term.Key, sc.Term.Quoted
		from, to = keySpan(toks, at, c, quoted)
	case sc.Key && at >= 0 && toks[at].Kind == jqsyntax.Key:
		program, known = sc.Input()
		typed = string([]rune(expr)[toks[at].Pos:c])
		from, to = toks[at].Pos, toks[at].End
	default:
		return 0, 0, nil, false
	}
	if !known {
		return cur, cur, nil, true
	}
	f := s.Shapes().Of(program)
	sh := f.Shape
	if sh != nil && sh.Count > 0 && sh.Kinds[doc.Array] == sh.Count && !quoted && !sc.Key {
		if sc.Term.Path != "" || sc.Term.Head != "" {
			from-- // after a path the [] form takes the dot's place: .projects[].tasks, not .projects.[].tasks
		}
		return off + from, off + to, elements(sh, typed, f.Cut), true
	}
	if sh == nil || sh.Kinds[doc.Object] == 0 {
		return cur, cur, nil, true
	}
	var exact, prefix, inside []*shape.Shape
	low := strings.ToLower(typed)
	for _, k := range sh.Keys() {
		switch {
		case k.Key == typed:
			exact = append(exact, k)
		case strings.HasPrefix(k.Key, typed):
			prefix = append(prefix, k)
		case strings.Contains(strings.ToLower(k.Key), low):
			inside = append(inside, k)
		}
	}
	for _, k := range append(append(exact, prefix...), inside...) {
		ins := shape.KeyText(k.Key)
		if quoted && !strings.HasPrefix(ins, `"`) {
			ins = `"` + ins + `"`
		}
		hint := ins + "  " + shape.Brief(k, 80)
		if n := sh.Kinds[doc.Object] - k.Count; n > 0 {
			hint += "  missing in " + plural(n, "object")
		}
		if f.Cut {
			hint += "  (of the first outputs)"
		}
		cs = append(cs, editor.Candidate{Insert: ins, Show: shape.KeyText(k.Key), Hint: hint})
	}
	return off + from, off + to, cs, true
}

// elements are what a field after an array can be, since a key is asked of its elements:
// on `.▮`, `[]` and `[0]`, then each element key as `[].key`; on `.em▮`, the element keys
// that match, so `.em<Tab>` at an array of {employee} gives `.[].employee` in one go.
func elements(sh *shape.Shape, typed string, cut bool) []editor.Candidate {
	e := sh.Elements()
	var cs []editor.Candidate
	of := ""
	if cut {
		of = "  (of the first outputs)"
	}
	if typed == "" {
		what := "nothing: the arrays are empty"
		if e != nil {
			what = shape.Brief(e, 80)
		}
		cs = append(cs,
			editor.Candidate{Insert: "[]", Hint: "[]  each element: " + what + of},
			editor.Candidate{Insert: "[0]", Hint: "[0]  the first element, [-1] the last; " + lenText(sh) + of})
	}
	if e == nil || e.Kinds[doc.Object] == 0 {
		return cs
	}
	low := strings.ToLower(typed)
	var prefix, inside []*shape.Shape
	for _, k := range e.Keys() {
		switch {
		case strings.HasPrefix(k.Key, typed):
			prefix = append(prefix, k)
		case strings.Contains(strings.ToLower(k.Key), low):
			inside = append(inside, k)
		}
	}
	for _, k := range append(prefix, inside...) {
		ins := "[]." + shape.KeyText(k.Key)
		cs = append(cs, editor.Candidate{Insert: ins, Hint: ins + "  " + shape.Brief(k, 80) + of})
	}
	return cs
}

// lenText is how long the arrays here are: `len 6`, `len 1..3`.
func lenText(sh *shape.Shape) string {
	if sh.MinLen == sh.MaxLen {
		return "len " + strconv.Itoa(sh.MinLen)
	}
	return "len " + strconv.Itoa(sh.MinLen) + ".." + strconv.Itoa(sh.MaxLen)
}

// keySpan is the part of the line a key replaces: after the dot of `.na▮`, the whole field
// when the cursor is inside it; in quotes, from the opening quote to the closing one, if any.
func keySpan(toks []jqsyntax.Token, at, cur int, quoted bool) (from, to int) {
	t := toks[at]
	if !quoted {
		if t.Kind == jqsyntax.Dot || t.Text == "." {
			return t.End, t.End
		}
		return t.Pos + 1, t.End
	}
	q := at
	for q > 0 && toks[q].Kind != jqsyntax.Quote {
		q--
	}
	to = cur
	for i := q + 1; i < len(toks); i++ {
		if k := toks[i].Kind; k != jqsyntax.Text && k != jqsyntax.Escape && k != jqsyntax.Quote {
			break
		}
		to = toks[i].End
		if toks[i].Kind == jqsyntax.Quote {
			break
		}
	}
	return toks[q].Pos, to
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}
