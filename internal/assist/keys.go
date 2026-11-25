package assist

import (
	"encoding/json"
	"regexp"
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
// object's key is being typed (Q-007, DESIGN §22), the values seen there when a value is
// (R-006), else Complete's names.
func CompleteOn(s *bench.Session) editor.Completer {
	return func(line string, cur int) (int, int, []editor.Candidate) {
		if from, to, cs, ok := editNames(s, line, cur); ok {
			return from, to, cs
		}
		if from, to, cs, ok := keys(s, line, cur); ok {
			return from, to, cs
		}
		if from, to, cs, ok := values(s, line, cur); ok {
			return from, to, cs
		}
		return Complete(line, cur)
	}
}

var editCmd = regexp.MustCompile(`^\s*:(?:e|edit)\s+`)

// editNames are what :e takes, after it: the loaded program's defs, named stages and main,
// by prefix, each with what it is as the hint.
func editNames(s *bench.Session, line string, cur int) (from, to int, cs []editor.Candidate, ok bool) {
	m := editCmd.FindString(line)
	rs := []rune(line)
	if m == "" || cur < len([]rune(m)) {
		return 0, 0, nil, false
	}
	from = len([]rune(m))
	prefix := string(rs[from:cur])
	for _, n := range s.EditNames() {
		if strings.HasPrefix(n.Name, prefix) {
			cs = append(cs, editor.Candidate{Insert: n.Name, Hint: n.About})
		}
	}
	return from, len(rs), cs, true
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
	case sc.Key && (at < 0 || between(toks[at])): // `{▮` or `{ id, ▮`: no key typed yet
		program, known = sc.Input()
		from, to = c, c
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
	var have map[string]bool // the literal's own keys, not offered again (Q-061)
	if sc.Key {
		have = literalKeys(expr, c)
	}
	var exact, prefix, inside, left []*shape.Shape
	low := strings.ToLower(typed)
	for _, k := range sh.Keys() {
		if have[k.Key] {
			continue
		}
		left = append(left, k)
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
	if sc.Key && typed == "" && len(left) > 1 { // last: Tab steps through the keys, shift+tab takes them all
		for i := range cs {
			cs[i].Hint += "  · shift+tab: every key"
		}
		cs = append(cs, everyKey(left, sh.Kinds[doc.Object]))
	}
	return off + from, off + to, cs, true
}

// maxEvery is how many keys every-key writes in at once: a wide object is cut, not dumped.
const maxEvery = 20

// everyKey is the offer that writes in every key left, as shorthand, in the document's order
// (Q-062): then delete what you don't want, rather than type what you do. It comes last, so
// the first Tab is one key, and shift+tab, which starts from the end, is all of them.
func everyKey(ks []*shape.Shape, objects int) editor.Candidate {
	n := len(ks)
	ks = ks[:min(n, maxEvery)]
	names := make([]string, len(ks))
	partial := 0
	for i, k := range ks {
		names[i] = shorthand(k.Key)
		if k.Count < objects {
			partial++
		}
	}
	text := strings.Join(names, ", ")
	hint := plural(n, "key") + " as shorthand: {" + text + "}"
	if n > maxEvery {
		hint = "the first " + strconv.Itoa(maxEvery) + " of " + plural(n, "key") + " as shorthand; a comma and Tab for the rest"
	}
	if partial > 0 {
		hint += "; " + plural(partial, "key") + " not in every object"
	}
	return editor.Candidate{Insert: text, Show: "… every key (" + strconv.Itoa(n) + ")", Hint: hint}
}

// shorthand is a key as an object entry that takes it from .: a plain name bare, anything
// else quoted ({"a b"} is {"a b": ."a b"}), keywords too, as older jq won't take {if}.
func shorthand(k string) string {
	if ts := jqsyntax.Lex(k); len(ts) == 1 && ts[0].Kind == jqsyntax.Keyword {
		q, _ := json.Marshal(k)
		return string(q)
	}
	return shape.KeyText(k)
}

// between says the cursor's token is where an object's next key starts: just after { or a
// comma, or in the space after them.
func between(t jqsyntax.Token) bool {
	return t.Kind == jqsyntax.Space || t.Text == "{" || t.Text == ","
}

// literalKeys are the keys already written in the object literal round cur, but for the
// entry being typed: `{ id, i▮ }` has id. Keys written as an expression ({(.k): 1}) aren't known.
func literalKeys(expr string, cur int) map[string]bool {
	var obj *jqsyntax.Node
	jqsyntax.Walk(jqsyntax.Parse(expr), func(n *jqsyntax.Node) bool {
		if n.Kind == jqsyntax.NObject && n.Pos < cur && (cur < n.End || n.Open) {
			obj = n // the innermost, as Walk goes down
		}
		return true
	})
	if obj == nil {
		return nil
	}
	have := map[string]bool{}
	for _, e := range obj.Kids {
		if e.Kind != jqsyntax.NEntry || len(e.Kids) == 0 || e.Pos <= cur && cur <= e.End {
			continue
		}
		switch k := e.Kids[0]; k.Kind {
		case jqsyntax.NKey:
			have[strings.TrimPrefix(k.Text, "$")] = true
		case jqsyntax.NString:
			var b strings.Builder
			for _, p := range k.Kids {
				if p.Kind != jqsyntax.NStrPart {
					b.Reset()
					break
				}
				b.WriteString(p.Text)
			}
			if b.Len() > 0 {
				have[b.String()] = true
			}
		}
	}
	return have
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
