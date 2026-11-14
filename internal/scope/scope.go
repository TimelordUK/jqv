// Package scope knows what `.` is at any point in a line of jq (Q-045, DESIGN §22), as a SQL
// editor knows its schema. At the cursor it gives the input, a jq program whose outputs are
// what `.` is there, run on the line's input; the term being typed (`.projects[0].na`); the
// variables in reach; and the frames, the stack of stages and constructs around the cursor,
// each with its own input, as a debugger shows a call stack.
//
// It reads the lexer's tokens (jqsyntax.Lex), not a parse tree, so a half-typed line is fine.
// It runs nothing: what the programs give is for the caller to find out, by walking the
// document's shape or running gojq.
package scope

import (
	"sort"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// Scope is what the line says about the cursor's place.
type Scope struct {
	Frames []Frame           // outermost first; the last is the one the cursor is in
	Vars   map[string]string // variables bound in reach, each as the program giving its values
	Term   Term              // the path being typed at the cursor
	Known  bool              // false in a def's body and wherever else . depends on a caller
	Key    bool              // the cursor is where an object's key goes: {na▮
	input  string            // the innermost frame's input
}

// Frame is a row of the stack: the line's input, a stage (`.[].employee`), a construct
// (`{…}`, `map(…)`), each with the program giving `.` there ("" when it isn't known).
type Frame struct {
	Label string
	Pos   int // where it starts in the line, in runes
	Input string
}

// Term is the path at the cursor. `.projects[0].na▮` is Path `.projects[0]`, Key `na`,
// Typing; `$e.x▮` has Head `$e`; `(.a | .b).c▮` has Head `(.a | .b)`. A term not started
// (after a space, an operator, a `(`) has Pos at the cursor and nothing else.
type Term struct {
	Pos    int
	Head   string // "", a variable, `..`, or a bracket the path starts from
	Path   string // after the head, up to the key being typed: `.projects[0]`
	Key    string // the key being typed, without its dot or quotes
	Typing bool   // the term ends in a field or a bare `.`, so a key is being typed
	Quoted bool   // the key is being typed in quotes: ."pa▮
}

// Input is the program whose outputs are `.` where the cursor is, on the line's input, and
// false where that isn't known (a def's body).
func (s Scope) Input() (string, bool) { return s.input, s.Known }

// Value is the program for what the term's path gives before the key being typed: the
// object whose keys a field after it can be. `.projects[0].na▮` on input `.[].employee`
// gives `.[].employee | .projects[0]`.
func (s Scope) Value() (string, bool) {
	t := s.Term
	if strings.HasPrefix(t.Head, "(") || strings.HasPrefix(t.Head, "[") || strings.HasSuffix(t.Head, ")") {
		if !s.Known {
			return "", false
		}
		return render([]part{{text: s.input}, {text: t.Head + t.Path}}), true // as written: [.a][0]
	}
	path := strings.TrimLeft(t.Path, "?") // $v? and ..? only quieten errors
	if strings.HasPrefix(path, "[") {
		path = "." + path
	}
	if t.Head != "" && t.Head[0] == '$' {
		base, ok := s.Vars[t.Head]
		if !ok {
			base = t.Head // $root, $_, $ENV: the session's own
		}
		return render([]part{{text: base}, {text: path}}), true
	}
	if !s.Known {
		return "", false
	}
	return render([]part{{text: s.input}, {text: t.Head}, {text: path}}), true
}

// part is a piece of a program: a stage, or `X as $v`, which binds and leaves `.` alone.
type part struct {
	text  string
	binds []string // the variables a binding part binds
}

// render joins parts with |, leaving out `.` and bindings no later part uses.
func render(ps []part) string {
	var keep []part
	for i, p := range ps {
		t := strings.TrimSpace(p.text)
		if t == "" || t == "." {
			continue
		}
		if len(p.binds) > 0 && !usedAfter(p.binds, ps[i+1:]) {
			continue
		}
		keep = append(keep, part{t, p.binds})
	}
	if len(keep) == 0 {
		return "."
	}
	var b strings.Builder
	for i, p := range keep {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(p.text)
	}
	if len(keep[len(keep)-1].binds) > 0 {
		b.WriteString(" | .")
	}
	return b.String()
}

func usedAfter(vars []string, ps []part) bool {
	for _, p := range ps {
		for _, t := range jqsyntax.Lex(p.text) {
			if t.Kind == jqsyntax.Variable {
				for _, v := range vars {
					if t.Text == v {
						return true
					}
				}
			}
		}
	}
	return false
}

// frame is one level of the walk: the top, a bracket, or an if or def at the same depth.
type frame struct {
	kind  byte   // 't' top, '(' group, '[' , '{', 'c' call, 'i' string \(, 'r' reduce/foreach (…), 'f' if, 'd' def
	label string // as the stack shows it
	pos   int
	base  []part // what . is where the frame starts
	known bool
	vars  map[string][]part

	parts   []part // stages done in this frame
	segPos  int    // where the stage being written starts
	itemPos int    // where the item being written starts: after a , as well
	lhs     string // the left side of a |= being written: its right side runs on it
	asPos   int    // where the binding `as` of this stage is, or -1
	asEnd   int
	labelAt int // where a `label $x` stage starts, or -1
	key     bool

	name  string   // a call's function, a def's name
	arg   int      // which argument of a call or reduce
	nargs int      // how many arguments the call has
	args  []string // the arguments written so far
	argAt int

	red   *reduction // a reduce or foreach waiting for its (
	redOf *reduction // a reduce or foreach frame's own
	body  bool       // a def frame: past its colon
}

type reduction struct {
	kw, src, pattern string
	srcPos, patPos   int
	vars             []string
}

func (f *frame) current() []part {
	ps := append(append([]part(nil), f.base...), f.parts...)
	if f.lhs != "" {
		ps = append(ps, part{text: f.lhs})
	}
	return ps
}

func (f *frame) reset(at int) {
	f.parts, f.lhs, f.segPos, f.itemPos, f.asPos, f.labelAt = nil, "", at, at, -1, -1
}

// At is what the line says about the cursor at cur, in runes.
func At(line string, cur int) Scope {
	rs := []rune(line)
	cur = min(max(cur, 0), len(rs))
	toks := jqsyntax.Lex(line)
	// text is the code from i to j, a comment taken as a space, since it would swallow what
	// follows it once the pieces are joined on one line
	text := func(i, j int) string {
		var b strings.Builder
		for k := sort.Search(len(toks), func(k int) bool { return toks[k].End > i }); k < len(toks) && toks[k].Pos < j; k++ {
			if toks[k].Kind == jqsyntax.Comment {
				b.WriteByte(' ')
			} else {
				b.WriteString(string(rs[max(toks[k].Pos, i):min(toks[k].End, j)]))
			}
		}
		return strings.TrimSpace(b.String())
	}

	term := findTerm(toks, rs, cur)
	limit := cur
	if term.Head != "" || term.Path != "" || term.Typing {
		limit = term.Pos
	}

	stack := []*frame{{kind: 't', label: ".", known: true, asPos: -1, labelAt: -1, vars: map[string][]part{}}}
	top := func() *frame { return stack[len(stack)-1] }
	push := func(f *frame) {
		f.asPos, f.labelAt, f.segPos, f.itemPos, f.argAt = -1, -1, f.pos, f.pos, f.pos
		if f.vars == nil {
			f.vars = map[string][]part{}
		}
		stack = append(stack, f)
	}
	inner := func(kind byte, label string, pos int, base []part, known bool) *frame {
		return &frame{kind: kind, label: label, pos: pos, base: base, known: known}
	}
	var prev jqsyntax.Token // the last code token
	for i, t := range toks {
		if t.End > limit {
			break
		}
		if t.Kind == jqsyntax.Space || t.Kind == jqsyntax.Comment {
			continue
		}
		f := top()
		switch {
		case t.Kind == jqsyntax.Pipe:
			s := text(f.segPos, t.Pos)
			switch {
			case f.asPos >= f.segPos:
				x := text(f.segPos, f.asPos)
				pattern := text(f.asEnd, t.Pos)
				vs := variables(pattern)
				f.parts = append(f.parts, part{text: s, binds: vs})
				for _, v := range vs {
					f.vars[v] = varProgram(f.current()[:len(f.base)+len(f.parts)-1], x, pattern, v)
				}
			case f.labelAt >= f.segPos:
				f.parts = append(f.parts, part{text: s, binds: variables(s)})
			case s != "":
				f.parts = append(f.parts, part{text: s})
			}
			f.lhs, f.segPos, f.itemPos, f.asPos, f.labelAt = "", t.End, t.End, -1, -1
		case t.Kind == jqsyntax.Operator && t.Text == "|=":
			f.lhs = text(f.itemPos, t.Pos)
		case t.Kind == jqsyntax.Punct && t.Text == ",":
			if f.kind == '{' {
				f.reset(t.End)
				f.key = true
			} else {
				f.lhs, f.itemPos = "", t.End
			}
		case t.Kind == jqsyntax.Punct && t.Text == ":":
			if f.kind == '{' {
				f.reset(t.End)
				f.key = false
			} else if f.kind == '[' { // a slice's bounds both run on the input
				f.reset(t.End)
			} else if f.kind == 'd' && !f.body {
				f.body = true
				f.reset(t.End)
			}
		case t.Kind == jqsyntax.Punct && t.Text == ";":
			switch f.kind {
			case 'd':
				stack = stack[:len(stack)-1]
				top().reset(t.End)
			case 'c', 'r':
				f.args = append(f.args, text(f.argAt, t.Pos))
				f.arg++
				f.argAt = t.End
				f.reset(t.End)
				if f.kind == 'c' {
					f.base, f.known = callBase(f.name, f.arg, f.nargs, f.args, stack[len(stack)-2])
				} else {
					f.base = reduceBase(f.redOf, f.arg, f.args, stack[len(stack)-2])
				}
			default:
				f.reset(t.End)
			}
		case t.Kind == jqsyntax.Keyword:
			switch t.Text {
			case "def":
				push(inner('d', "def", t.Pos, nil, false))
			case "if":
				push(inner('f', "if", t.Pos, f.current(), f.known))
			case "then", "elif", "else":
				if f.kind == 'f' {
					f.reset(t.End)
					f.label = t.Text
				}
			case "end":
				if f.kind == 'f' {
					stack = stack[:len(stack)-1]
				}
			case "reduce", "foreach":
				f.red = &reduction{kw: t.Text, srcPos: t.End}
			case "as":
				if f.red != nil && f.red.src == "" {
					f.red.src, f.red.patPos = text(f.red.srcPos, t.Pos), t.End
				} else {
					f.asPos, f.asEnd = t.Pos, t.End
				}
			case "label":
				f.labelAt = t.Pos
			}
		case t.Kind == jqsyntax.Name && f.kind == 'd' && f.name == "" && prev.Text == "def":
			f.name = t.Text
			f.label = "def " + t.Text
		case t.Kind == jqsyntax.Bracket && t.Text == "(" || t.Kind == jqsyntax.Interp && t.Text == `\(`:
			switch {
			case t.Kind == jqsyntax.Interp:
				push(inner('i', `"\(…)"`, t.End, f.current(), f.known))
			case prev.Kind == jqsyntax.Name:
				c := inner('c', "", t.End, nil, f.known)
				c.name, c.nargs = prev.Text, countArgs(toks, i)
				if f.kind == 'd' && !f.body {
					c.known = false // a def's parameters
				} else {
					c.base, c.known = callBase(c.name, 0, c.nargs, nil, f)
				}
				push(c)
			case f.red != nil && f.red.src != "":
				r := f.red
				r.pattern = text(r.patPos, t.Pos)
				r.vars = variables(r.pattern)
				f.red = nil
				c := inner('r', "", t.End, f.current(), f.known)
				c.redOf, c.nargs = r, countArgs(toks, i)
				c.vars = map[string][]part{}
				for _, v := range r.vars {
					c.vars[v] = varProgram(f.current(), r.src, r.pattern, v)
				}
				push(c)
			default:
				push(inner('(', "(…)", t.End, f.current(), f.known))
			}
		case t.Kind == jqsyntax.Bracket && t.Text == "[":
			push(inner('[', "[…]", t.End, f.current(), f.known))
		case t.Kind == jqsyntax.Bracket && t.Text == "{":
			o := inner('{', "{…}", t.End, f.current(), f.known)
			o.key = true
			push(o)
		case t.Kind == jqsyntax.Bracket || t.Kind == jqsyntax.Interp: // a closer
			for len(stack) > 1 {
				k := top().kind
				stack = stack[:len(stack)-1]
				if k != 'f' && k != 'd' {
					break
				}
			}
		}
		prev = t
	}

	s := Scope{Term: term, Vars: map[string]string{}}
	s.Frames = append(s.Frames, Frame{Label: ".", Input: "."})
	for n, f := range stack {
		if n > 0 {
			in := ""
			if f.known {
				in = render(f.base)
			}
			label := f.label
			if f.kind == 'c' || f.kind == 'r' {
				label = argLabel(f) // here, not on every ;, which on a long call is slow
			}
			s.Frames = append(s.Frames, Frame{Label: label, Pos: f.pos, Input: in})
		}
		for j := range f.parts {
			in := ""
			if f.known {
				in = render(append(append([]part(nil), f.base...), f.parts[:j+1]...))
			}
			s.Frames = append(s.Frames, Frame{Label: f.parts[j].text, Input: in})
		}
		if f.lhs != "" {
			in := ""
			if f.known {
				in = render(f.current())
			}
			s.Frames = append(s.Frames, Frame{Label: f.lhs + " |=", Input: in})
		}
		for v, ps := range f.vars {
			if f.known {
				s.Vars[v] = render(ps)
			}
		}
	}
	f := top()
	s.Known, s.Key = f.known, f.kind == '{' && f.key
	if s.Known {
		s.input = render(f.current())
	}
	return s
}

// findTerm reads back from the cursor over the path being typed.
func findTerm(toks []jqsyntax.Token, rs []rune, cur int) Term {
	k := -1
	for i, t := range toks {
		if t.Pos < cur {
			k = i
		}
	}
	none := Term{Pos: cur}
	if k < 0 {
		return none
	}
	at := toks[k]
	tail := string(rs[at.Pos:cur]) // the last token as far as the cursor
	t := Term{}
	end := k // the tokens k+1.. are the key being typed, k.. the path's last
	switch {
	case at.Kind == jqsyntax.Field && tail != ".":
		t.Typing, t.Key = true, tail[1:]
		end = k - 1
	case at.Kind == jqsyntax.Dot || at.Kind == jqsyntax.Field:
		t.Typing = true
		end = k - 1
	case at.Kind == jqsyntax.Text && k >= 2 && toks[k-1].Kind == jqsyntax.Quote && toks[k-2].Kind == jqsyntax.Field && toks[k-2].Text == "." && !closed(toks, k-1, cur):
		t.Typing, t.Quoted, t.Key = true, true, tail
		end = k - 3
	case at.Kind == jqsyntax.Quote && k >= 1 && toks[k-1].Kind == jqsyntax.Field && toks[k-1].Text == "." && at.End == cur && !closed(toks, k, cur):
		t.Typing, t.Quoted = true, true
		end = k - 2
	}
	start := end + 1
	head := ""
	opaque := false // the path follows something it can't be read from
	for j := end; j >= 0; {
		u := toks[j]
		switch {
		case u.Kind == jqsyntax.Field || u.Kind == jqsyntax.Dot || u.Kind == jqsyntax.Operator && u.Text == "?":
			start, j = j, j-1
			continue
		case u.Kind == jqsyntax.Recurse:
			start, head = j, ".."
		case u.Kind == jqsyntax.Variable:
			start, head = j, u.Text
		case u.Kind == jqsyntax.Quote && j >= 2:
			o := j - 1
			for o >= 0 && toks[o].Kind != jqsyntax.Quote {
				if toks[o].Kind == jqsyntax.Interp {
					o = -1
				}
				o--
			}
			if o >= 1 && toks[o-1].Kind == jqsyntax.Field && toks[o-1].Text == "." {
				start, j = o-1, o-2
				continue
			}
		case u.Kind == jqsyntax.Bracket && (u.Text == "]" || u.Text == ")"):
			o := opener(toks, j)
			if o < 0 {
				break
			}
			if p := prevCode(toks, o); u.Text == "]" && p >= 0 && pathEnd(toks[p]) {
				start, j = o, p
				continue
			}
			if p := prevCode(toks, o); u.Text == "]" && p >= 0 && literal(toks[p]) {
				opaque = true // `0[1]`, `null[0]`: indexing what isn't a path
				break
			}
			if p := prevCode(toks, o); u.Text == ")" && p >= 0 && (toks[p].Kind == jqsyntax.Variable || toks[p].Text == "]" || toks[p].Text == "}") {
				opaque = true // a reduce or foreach's (…), after its pattern: not a path's start
				break
			}
			start = o
			if p := prevCode(toks, o); u.Text == ")" && p >= 0 && toks[p].Kind == jqsyntax.Name {
				start = p
			}
			head = string(rs[toks[start].Pos:u.End])
		}
		break
	}
	if opaque {
		return none
	}
	if head == "" && start <= end && toks[start].Kind != jqsyntax.Field && toks[start].Kind != jqsyntax.Dot {
		start = end + 1 // `0?`: what the path follows isn't one
		if t.Typing {
			return none
		}
	}
	if start > end {
		if !t.Typing {
			return none
		}
		if end+1 < len(toks) {
			t.Pos = toks[end+1].Pos
		}
		return t
	}
	t.Pos = toks[start].Pos
	pathFrom := t.Pos + len([]rune(head))
	pathTo := toks[end].End
	if pathTo > pathFrom {
		t.Path = string(rs[pathFrom:pathTo])
	}
	t.Head = head
	return t
}

// pathEnd is whether a [ after t indexes it rather than starting an array.
func pathEnd(t jqsyntax.Token) bool {
	switch t.Kind {
	case jqsyntax.Field, jqsyntax.Dot, jqsyntax.Recurse, jqsyntax.Variable, jqsyntax.Quote:
		return true
	case jqsyntax.Bracket:
		return t.Text == "]" || t.Text == ")"
	case jqsyntax.Operator:
		return t.Text == "?"
	}
	return false
}

// literal is whether t ends a value that isn't a path: a number, a string, a call, a format,
// null, true or false.
func literal(t jqsyntax.Token) bool {
	switch t.Kind {
	case jqsyntax.Number, jqsyntax.Quote, jqsyntax.Name, jqsyntax.Format:
		return true
	case jqsyntax.Keyword:
		return t.Text == "null" || t.Text == "true" || t.Text == "false"
	}
	return false
}

// closed is whether the string opened at toks[q] is closed before cur.
func closed(toks []jqsyntax.Token, q, cur int) bool {
	for _, t := range toks[q+1:] {
		if t.Pos >= cur {
			return false
		}
		if t.Kind == jqsyntax.Quote {
			return true
		}
	}
	return false
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

// opener is the index of the bracket that the closer at toks[j] closes, or -1.
func opener(toks []jqsyntax.Token, j int) int {
	for o := j - 1; o >= 0; o-- {
		if toks[o].Kind == jqsyntax.Bracket && toks[o].Depth == toks[j].Depth {
			return o
		}
	}
	return -1
}

// countArgs is how many arguments the call whose ( is toks[i] has, read ahead to its ).
func countArgs(toks []jqsyntax.Token, i int) int {
	n, d := 1, toks[i].Depth
	for _, t := range toks[i+1:] {
		if t.Depth == d && (t.Kind == jqsyntax.Bracket || t.Kind == jqsyntax.Interp) {
			break
		}
		if t.Depth == d+1 && t.Kind == jqsyntax.Punct && t.Text == ";" {
			n++
		}
	}
	return n
}

// variables are the $names in a pattern: `$x`, `[$a, $b]`, `{$k, v: $w}`.
func variables(pattern string) []string {
	var vs []string
	for _, t := range jqsyntax.Lex(pattern) {
		if t.Kind == jqsyntax.Variable {
			vs = append(vs, t.Text)
		}
	}
	return vs
}

// varProgram is the program giving $v's values: ctx | x for a plain pattern, else the
// destructuring itself, ctx | x as pattern | $v.
func varProgram(ctx []part, x, pattern, v string) []part {
	ps := append([]part(nil), ctx...)
	if strings.TrimSpace(pattern) == v {
		return append(ps, part{text: x})
	}
	return append(ps, part{text: x + " as " + pattern, binds: variables(pattern)}, part{text: v})
}

// callBase is what . is in argument arg of a call to name, from the frame it is written in
// (DESIGN §22's table): each element for map and the _by functions, the entries for
// with_entries, the first argument's outputs for any/all/INDEX's second, nothing known for
// sub's replacement (its . is the captures); otherwise the call's input.
func callBase(name string, arg, nargs int, args []string, f *frame) ([]part, bool) {
	cur := f.current()
	with := func(t string) []part { return append(cur, part{text: t}) }
	switch name {
	case "map", "map_values", "sort_by", "group_by", "unique_by", "min_by", "max_by":
		if arg == 0 {
			return with(".[]"), f.known
		}
	case "any", "all", "INDEX":
		if nargs == 1 {
			return with(".[]"), f.known
		}
		if arg == 1 && len(args) > 0 {
			return with(args[0]), f.known
		}
	case "with_entries":
		return with("to_entries[]"), f.known
	case "walk", "paths":
		return with(".."), f.known
	case "sub", "gsub":
		if arg == 1 {
			return nil, false
		}
	}
	return cur, f.known
}

// reduceBase is what . is in a reduce or foreach's (init; update; extract): the input in
// init; after it, the accumulator, taken to be what init gives, with the variable bound.
func reduceBase(r *reduction, arg int, args []string, f *frame) []part {
	cur := f.current()
	if arg == 0 || len(args) == 0 {
		return cur
	}
	return append(cur, part{text: r.src + " as " + r.pattern, binds: r.vars}, part{text: args[0]})
}

// argLabel is a call or reduce as the stack shows it, ▮ in the argument the cursor is in:
// `map(▮)`, `limit(…; ▮)`, `reduce .[] as $x (▮; …)`.
func argLabel(f *frame) string {
	slots := func(name string) string {
		as := make([]string, max(f.nargs, f.arg+1))
		for i := range as {
			as[i] = "…"
		}
		as[f.arg] = "▮"
		return name + "(" + strings.Join(as, "; ") + ")"
	}
	if f.redOf != nil {
		return slots(f.redOf.kw + " " + f.redOf.src + " as " + f.redOf.pattern + " ")
	}
	return slots(f.name)
}
