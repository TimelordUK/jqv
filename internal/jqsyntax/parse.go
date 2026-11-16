package jqsyntax

import (
	"strings"
)

// NodeKind is what a Node is: a piece of jq's grammar (gojq's parser.go.y).
type NodeKind int

const (
	NMissing   NodeKind = iota // an operand still to come: `.a | ` ends in one; no width
	NBad                       // a token where nothing fits, or tokens skipped to a closer
	NBroken                    // pieces that don't join: a tree, the Bad after it, and on
	NPipe                      // a | b
	NComma                     // a, b
	NBinary                    // a op b; Text is the op: // = |= += or and == < + * …
	NBind                      // src as $x ?// [$y] | body: Kids [src, patterns…, body]
	NDefs                      // def …; def …; query: Kids [Def…, query]; no query in a file of defs alone
	NDef                       // def name(params): body; Text the name, Kids [Param…, body]
	NParam                     // a def's parameter: f or $x
	NIdentity                  // .
	NRecurse                   // ..
	NField                     // .a, or ."a" with Kids [String]; also a suffix
	NLiteral                   // null true false
	NNumber                    // 1 .5 1e3
	NString                    // "…": Kids StrPart, Interp and Bad, in order
	NStrPart                   // a string's text between its quotes and \( ), as written
	NInterp                    // \( query ): Kids [query]
	NFormat                    // @csv, or @csv "…" with Kids [String]
	NVar                       // $x, $__loc__, $mod::x; also a pattern
	NCall                      // name, name(a; b): Text the name, Kids the arguments
	NParen                     // ( query )
	NArray                     // [ query ], [] with no Kids
	NObject                    // { Entry, … }
	NEntry                     // key, or key: value; also in a pattern
	NKey                       // an object's key written as a name, keyword or $x
	NUnary                     // -term, +term: Text the sign
	NIf                        // Kids [cond, then, (elif cond, then)…, else?]
	NTry                       // try body catch handler: Kids [body, handler?]
	NReduce                    // reduce src as pattern (init; update)
	NForeach                   // foreach src as pattern (init; update; extract?)
	NLabel                     // label $x | body: Text $x
	NBreak                     // break $x: Text $x
	NPostfix                   // a term and its suffixes: Kids [term, Field|Index|Slice|Iterate|Optional…]
	NIndex                     // [query] after a term
	NSlice                     // [from:to] after a term: Kids [from, to], either Empty
	NIterate                   // [] after a term
	NOptional                  // ? after a term
	NEmpty                     // a slice's bound left out
	NPatArray                  // [pattern, …]
	NPatObject                 // {Entry, …}: an Entry's value is a pattern
	NModule                    // module, import and include, then the body: Kids [Directive…, body?]
	NDirective                 // module {…}; import "p" as name {…}; include "p" {…};
)

var nodeNames = [...]string{"missing", "bad", "broken", "pipe", "comma", "binary", "bind", "defs",
	"def", "param", "identity", "recurse", "field", "literal", "num", "string", "strpart", "interp",
	"format", "var", "call", "paren", "array", "object", "entry", "key", "unary", "if", "try",
	"reduce", "foreach", "label", "break", "postfix", "index", "slice", "iterate", "optional",
	"empty", "patarray", "patobject", "module", "directive"}

func (k NodeKind) String() string { return nodeNames[k] }

// Node is a piece of a jq program's tree, Pos to End in runes, from its first token to its
// last. Kids are in the order they are in the text. Open is a node whose closer hasn't come
// (a bracket, a string, if's end, a def's ;, as's |): the text is unfinished there, and the
// node may lack its later Kids.
type Node struct {
	Kind     NodeKind
	Pos, End int
	Text     string // the name, operator, keyword, variable or literal, as written
	Kids     []*Node
	Open     bool
}

// Parse reads jq text into a tree (Q-048, DESIGN §23). Like Lex it never fails: half-typed
// text gives a tree with Missing nodes where an operand is still to come and Open nodes where
// a closer is, and a token where nothing fits is a Bad node. On text gojq parses, the tree is
// gojq's, with positions (FuzzParse holds it to that).
func Parse(text string) *Node {
	p := &parser{rs: []rune(text), refused: -1}
	p.end = len(p.rs)
	for _, t := range Lex(text) {
		if t.Kind != Space && t.Kind != Comment {
			p.ts = append(p.ts, t)
		}
	}
	root := p.module()
	for p.i < len(p.ts) { // what is left doesn't join on: a stray ), a term after a term
		t := p.next()
		root = broken(root, &Node{Kind: NBad, Pos: t.Pos, End: t.End, Text: t.Text})
		if rest := p.pipe(); rest.Kind != NMissing {
			root = broken(root, rest)
		}
	}
	return root
}

type parser struct {
	rs   []rune
	ts   []Token // Lex's, without spaces and comments
	i    int     // the next token
	prev int     // where the last token taken ends
	end  int
	lib  int // where a body of defs alone, a library, may start
	// refused is the operator that would chain an assignment or comparison, for no outer
	// level to take either
	refused int
}

func (p *parser) peek() (Token, bool) {
	if p.i < len(p.ts) {
		return p.ts[p.i], true
	}
	return Token{}, false
}

func (p *parser) peekAt(k int) (Token, bool) {
	if p.i+k < len(p.ts) {
		return p.ts[p.i+k], true
	}
	return Token{}, false
}

func (p *parser) is(k Kind, text string) bool {
	t, ok := p.peek()
	return ok && t.Kind == k && t.Text == text
}

func (p *parser) isKind(k Kind) bool {
	t, ok := p.peek()
	return ok && t.Kind == k
}

// isLocal is a name or variable of the next token's kind, not a module's (mod::f): where a
// def, parameter, key, pattern or label is named.
func (p *parser) isLocal(k Kind) bool {
	t, ok := p.peek()
	return ok && t.Kind == k && !module(t)
}

func module(t Token) bool { return strings.Contains(t.Text, "::") }

func (p *parser) next() Token {
	t := p.ts[p.i]
	p.i++
	p.prev = t.End
	return t
}

func (p *parser) take(k Kind, text string) bool {
	if p.is(k, text) {
		p.next()
		return true
	}
	return false
}

// open starts a node at the next token.
func (p *parser) open(k NodeKind, text string) *Node {
	pos := p.end
	if t, ok := p.peek(); ok {
		pos = t.Pos
	}
	return &Node{Kind: k, Pos: pos, Text: text}
}

// opener starts a node at the next token and takes it, its text the node's.
func (p *parser) opener(k NodeKind) *Node {
	n := p.open(k, "")
	n.Text = p.next().Text
	return n
}

// done ends n at the last token taken.
func (p *parser) done(n *Node) *Node {
	if p.prev < n.Pos { // nothing taken
		n.Pos = p.prev
	}
	if len(n.Kids) > 0 && n.Kids[0].Pos < n.Pos { // it starts with a Missing, back where the last token ended
		n.Pos = n.Kids[0].Pos
	}
	n.End = p.prev
	return n
}

func (p *parser) unclosed(n *Node) *Node {
	n.Open = true
	return p.done(n)
}

func (p *parser) missing() *Node { return &Node{Kind: NMissing, Pos: p.prev, End: p.prev} }

// leaf takes the next token as a node of its own.
func (p *parser) leaf(k NodeKind) *Node {
	t := p.next()
	return &Node{Kind: k, Pos: t.Pos, End: t.End, Text: t.Text}
}

func (p *parser) join(k NodeKind, text string, kids ...*Node) *Node {
	return p.done(&Node{Kind: k, Pos: kids[0].Pos, Text: text, Kids: kids})
}

func broken(a, b *Node) *Node {
	if a.Kind == NBroken {
		a.Kids = append(a.Kids, b)
		a.End = b.End
		return a
	}
	return &Node{Kind: NBroken, Pos: a.Pos, End: b.End, Kids: []*Node{a, b}}
}

// close takes the closer of open, a bracket or a string's \(. Tokens before it that fit
// nothing are a Bad node, joined to n's last kid; with no closer to come, n is Open.
func (p *parser) close(n *Node, open Token, closer string) *Node {
	shut := func(t Token) bool {
		return t.Text == closer && t.Depth == open.Depth && (t.Kind == Bracket || t.Kind == Interp)
	}
	if t, ok := p.peek(); ok && shut(t) {
		p.next()
		return p.done(n)
	}
	k := p.i
	for k < len(p.ts) && p.ts[k].Depth > open.Depth {
		k++
	}
	if k == len(p.ts) || !shut(p.ts[k]) || k == p.i {
		return p.unclosed(n)
	}
	from, to := p.ts[p.i].Pos, p.ts[k-1].End
	bad := &Node{Kind: NBad, Pos: from, End: to, Text: string(p.rs[from:to])}
	if last := len(n.Kids) - 1; last >= 0 {
		n.Kids[last] = broken(n.Kids[last], bad)
	} else {
		n.Kids = append(n.Kids, bad)
	}
	p.i = k
	p.next()
	return p.done(n)
}

// module is the whole text: module, import and include, then the body.
func (p *parser) module() *Node {
	var dirs []*Node
	for (len(dirs) == 0 && p.is(Keyword, "module")) || p.is(Keyword, "import") || p.is(Keyword, "include") {
		d := p.directive()
		dirs = append(dirs, d)
		if d.Open {
			break
		}
	}
	p.lib = p.i
	var body *Node
	if p.i == len(p.ts) {
		if len(dirs) == 0 { // nothing but spaces and comments: a program with nothing in it
			return &Node{Kind: NDefs, Pos: 0, End: 0}
		}
	} else {
		body = p.pipe()
	}
	if len(dirs) == 0 {
		return body
	}
	n := &Node{Kind: NModule, Pos: dirs[0].Pos, Kids: dirs}
	if body != nil {
		n.Kids = append(n.Kids, body)
	}
	return p.done(n)
}

func (p *parser) directive() *Node {
	n := p.opener(NDirective)
	if n.Text != "module" {
		if !p.isKind(Quote) {
			return p.unclosed(n)
		}
		n.Kids = append(n.Kids, p.plain())
		if n.Text == "import" {
			if !p.take(Keyword, "as") {
				return p.unclosed(n)
			}
			if !p.isLocal(Name) && !p.isLocal(Variable) {
				return p.unclosed(n)
			}
			n.Kids = append(n.Kids, p.leaf(NKey))
		}
	}
	if p.is(Bracket, "{") {
		n.Kids = append(n.Kids, p.constant())
	} else if n.Text == "module" {
		return p.unclosed(n)
	}
	if !p.take(Punct, ";") {
		return p.unclosed(n)
	}
	return p.done(n)
}

// constant is a directive's meta and what is in it: JSON written as jq, keys as names, no
// queries. An array's Kids are its elements.
func (p *parser) constant() *Node {
	t, ok := p.peek()
	switch {
	case !ok:
	case t.Kind == Number:
		return p.leaf(NNumber)
	case t.Kind == Keyword && (t.Text == "null" || t.Text == "true" || t.Text == "false"):
		return p.leaf(NLiteral)
	case t.Kind == Quote:
		return p.plain()
	case t.Kind == Bracket && t.Text == "[":
		n := p.open(NArray, "")
		open := p.next()
		if !p.is(Bracket, "]") {
			n.Kids = append(n.Kids, p.constant())
			for p.take(Punct, ",") {
				n.Kids = append(n.Kids, p.constant())
			}
		}
		return p.close(n, open, "]")
	case t.Kind == Bracket && t.Text == "{":
		n := p.open(NObject, "")
		open := p.next()
		for !p.is(Bracket, "}") {
			e := p.open(NEntry, "")
			if t, ok := p.peek(); ok && (t.Kind == Key || t.Kind == Keyword || t.Kind == Name) && !module(t) {
				e.Kids = append(e.Kids, p.leaf(NKey))
			} else if ok && t.Kind == Quote {
				e.Kids = append(e.Kids, p.plain())
			} else {
				e.Kids = append(e.Kids, p.missing())
			}
			if p.take(Punct, ":") {
				e.Kids = append(e.Kids, p.constant())
			} else {
				e.Kids = append(e.Kids, p.missing())
			}
			n.Kids = append(n.Kids, p.done(e))
			if !p.take(Punct, ",") {
				break
			}
		}
		return p.close(n, open, "}")
	}
	return p.missing()
}

// plain is a string with no \( ) in it, as a path or a constant must be.
func (p *parser) plain() *Node {
	s := p.str()
	for _, k := range s.Kids {
		if k.Kind == NInterp && !s.Open {
			return &Node{Kind: NBad, Pos: s.Pos, End: s.End, Text: string(p.rs[s.Pos:s.End])}
		}
	}
	return s
}

// pipe is a query: a | b, loosest of all, to the right.
func (p *parser) pipe() *Node {
	l := p.comma()
	if p.isKind(Pipe) {
		p.next()
		return p.join(NPipe, "", l, p.pipe())
	}
	return l
}

func (p *parser) comma() *Node {
	l := p.operand()
	for p.take(Punct, ",") {
		l = p.join(NComma, "", l, p.operand())
	}
	return l
}

// operand is a comma's operand: an expression, or one bound with as, whose body runs to the
// end of the query.
func (p *parser) operand() *Node {
	switch { // def and label start a query, not an expression: `1 + def …` isn't jq
	case p.is(Keyword, "def"):
		return p.defs()
	case p.is(Keyword, "label"):
		return p.label()
	}
	e := p.binary(1)
	if !p.is(Keyword, "as") {
		return e
	}
	p.next()
	n := &Node{Kind: NBind, Pos: e.Pos, Kids: []*Node{e, p.pattern()}}
	for p.take(Operator, "?//") {
		n.Kids = append(n.Kids, p.pattern())
	}
	if !p.isKind(Pipe) {
		return p.unclosed(n)
	}
	p.next()
	n.Kids = append(n.Kids, p.pipe())
	return p.done(n)
}

// levels are the binary operators' precedence, loosest first: // is to the right, the
// assignments and comparisons don't chain, the rest are to the left.
var levels = map[string]int{
	"//": 1,
	"=":  2, "|=": 2, "+=": 2, "-=": 2, "*=": 2, "/=": 2, "%=": 2, "//=": 2,
	"or":  3,
	"and": 4,
	"==":  5, "!=": 5, "<": 5, "<=": 5, ">": 5, ">=": 5,
	"+": 6, "-": 6,
	"*": 7, "/": 7, "%": 7,
}

func (p *parser) binary(min int) *Node {
	l := p.postfix()
	chained := 0 // an assignment's or comparison's level, which can't take another of its own
	for {
		t, ok := p.peek()
		if !ok || (t.Kind != Operator && t.Kind != Keyword) || p.i == p.refused {
			return l
		}
		lv := levels[t.Text]
		if lv == 0 || lv < min {
			return l
		}
		if lv == chained { // 1 < 2 < 3: no level takes the second, so it is left over
			p.refused = p.i
			return l
		}
		if lv == 2 || lv == 5 {
			chained = lv
		}
		p.next()
		next := lv + 1
		if lv == 1 {
			next = lv
		}
		l = p.join(NBinary, t.Text, l, p.binary(next))
	}
}

// postfix is a term and what follows it: .a, [i], [a:b], [], ?, ."a".
func (p *parser) postfix() *Node {
	b := p.primary()
	if b.Kind == NMissing {
		return b
	}
	var n *Node
	for {
		s := p.suffix()
		if s == nil {
			break
		}
		if n == nil {
			n = &Node{Kind: NPostfix, Pos: b.Pos, Kids: []*Node{b}}
		}
		n.Kids = append(n.Kids, s)
	}
	if n == nil {
		return b
	}
	return p.done(n)
}

func (p *parser) suffix() *Node {
	t, ok := p.peek()
	if !ok {
		return nil
	}
	switch {
	case t.Kind == Field:
		return p.field()
	case t.Kind == Dot:
		n, ok := p.peekAt(1)
		switch {
		case ok && n.Kind == Quote:
			return p.field()
		case ok && n.Kind == Bracket && n.Text == "[":
			start := p.next().Pos
			s := p.bracket()
			s.Pos = start
			return s
		case !ok: // `.a.` at the end: a [ or a string to come
			s := p.leaf(NField)
			s.Kids = append(s.Kids, p.missing())
			return s
		}
	case t.Kind == Bracket && t.Text == "[":
		return p.bracket()
	case t.Kind == Operator && t.Text == "?":
		return p.leaf(NOptional)
	}
	return nil
}

// field is .a, or ."a" (a Field or Dot then a string).
func (p *parser) field() *Node {
	t := p.next()
	n := &Node{Kind: NField, Pos: t.Pos, Text: t.Text}
	if t.Kind == Dot || t.Text == "." {
		n.Text = "."
		n.Kids = append(n.Kids, p.str())
	}
	return p.done(n)
}

// bracket is a suffix's [i], [a:b], [a:], [:b] or [].
func (p *parser) bracket() *Node {
	n := p.open(NIndex, "")
	open := p.next()
	if t, ok := p.peek(); ok && t.Kind == Bracket && t.Text == "]" && t.Depth == open.Depth {
		p.next()
		n.Kind = NIterate
		return p.done(n)
	}
	empty := func() *Node { return &Node{Kind: NEmpty, Pos: p.prev, End: p.prev} }
	var from *Node
	if p.is(Punct, ":") {
		from = empty()
	} else {
		from = p.pipe()
	}
	n.Kids = append(n.Kids, from)
	if p.take(Punct, ":") {
		n.Kind = NSlice
		if from.Kind != NEmpty && p.is(Bracket, "]") {
			n.Kids = append(n.Kids, empty())
		} else {
			n.Kids = append(n.Kids, p.pipe())
		}
	}
	return p.close(n, open, "]")
}

func (p *parser) primary() *Node {
	t, ok := p.peek()
	if !ok {
		return p.missing()
	}
	switch t.Kind {
	case Dot:
		if n, ok := p.peekAt(1); ok && n.Kind == Quote {
			return p.field()
		}
		return p.leaf(NIdentity)
	case Field:
		return p.field()
	case Recurse:
		return p.leaf(NRecurse)
	case Number:
		return p.leaf(NNumber)
	case Quote:
		return p.str()
	case Format:
		n := p.opener(NFormat)
		if p.isKind(Quote) {
			n.Kids = append(n.Kids, p.str())
		}
		return p.done(n)
	case Variable:
		return p.leaf(NVar)
	case Name, Key:
		return p.call()
	case Keyword:
		switch t.Text {
		case "null", "true", "false":
			return p.leaf(NLiteral)
		case "if":
			return p.ifThen()
		case "try":
			n := p.opener(NTry)
			n.Kids = append(n.Kids, p.postfix())
			if p.take(Keyword, "catch") {
				n.Kids = append(n.Kids, p.postfix())
			}
			return p.done(n)
		case "reduce", "foreach":
			return p.fold()
		case "break":
			n := p.open(NBreak, "")
			p.next()
			if !p.isLocal(Variable) {
				return p.unclosed(n)
			}
			n.Text = p.next().Text
			return p.done(n)
		}
	case Bracket:
		switch t.Text {
		case "(":
			return p.paren()
		case "[":
			n := p.open(NArray, "")
			open := p.next()
			if t, ok := p.peek(); ok && t.Kind == Bracket && t.Text == "]" && t.Depth == open.Depth {
				p.next()
				return p.done(n)
			}
			n.Kids = append(n.Kids, p.pipe())
			return p.close(n, open, "]")
		case "{":
			return p.object()
		}
	case Operator:
		if t.Text == "-" || t.Text == "+" {
			n := p.opener(NUnary)
			n.Kids = append(n.Kids, p.postfix())
			return p.done(n)
		}
	}
	return p.missing()
}

// label is label $x | body.
func (p *parser) label() *Node {
	n := p.open(NLabel, "")
	p.next()
	if !p.isLocal(Variable) {
		return p.unclosed(n)
	}
	n.Text = p.next().Text
	if !p.isKind(Pipe) {
		return p.unclosed(n)
	}
	p.next()
	n.Kids = append(n.Kids, p.pipe())
	return p.done(n)
}

func (p *parser) paren() *Node {
	n := p.open(NParen, "")
	open := p.next()
	n.Kids = append(n.Kids, p.pipe())
	return p.close(n, open, ")")
}

// call is name, or name(a; b).
func (p *parser) call() *Node {
	n := p.opener(NCall)
	if !p.is(Bracket, "(") {
		return p.done(n)
	}
	open := p.next()
	n.Kids = append(n.Kids, p.pipe())
	for p.take(Punct, ";") {
		n.Kids = append(n.Kids, p.pipe())
	}
	return p.close(n, open, ")")
}

// str is a string from its opening quote: its text, its \( ) and its closing quote.
func (p *parser) str() *Node {
	n := p.open(NString, "")
	p.next()
	var part *Node
	for {
		t, ok := p.peek()
		if !ok {
			return p.unclosed(n)
		}
		switch t.Kind {
		case Text, Escape:
			p.next()
			if part == nil {
				part = &Node{Kind: NStrPart, Pos: t.Pos}
				n.Kids = append(n.Kids, part)
			}
			part.End = t.End
			part.Text += t.Text
		case Interp:
			part = nil
			in := p.open(NInterp, "")
			open := p.next()
			in.Kids = append(in.Kids, p.pipe())
			n.Kids = append(n.Kids, p.close(in, open, ")"))
		case Quote:
			p.next()
			return p.done(n)
		default: // an escape gojq won't read, or what is left in a \( that didn't close
			part = nil
			n.Kids = append(n.Kids, p.leaf(NBad))
		}
	}
}

func (p *parser) object() *Node {
	n := p.open(NObject, "")
	open := p.next()
	for {
		if t, ok := p.peek(); !ok || (t.Kind == Bracket && t.Text == "}" && t.Depth == open.Depth) {
			break
		}
		n.Kids = append(n.Kids, p.entry())
		if !p.take(Punct, ",") {
			break
		}
	}
	return p.close(n, open, "}")
}

// entry is an object's key, or key: value, where the value is expressions piped, no commas.
func (p *parser) entry() *Node {
	n := p.open(NEntry, "")
	key := p.key()
	n.Kids = append(n.Kids, key)
	if p.take(Punct, ":") {
		n.Kids = append(n.Kids, p.value())
	} else if key.Kind == NParen { // {(k)} has no value to take
		n.Kids = append(n.Kids, p.missing())
	}
	return p.done(n)
}

func (p *parser) key() *Node {
	t, ok := p.peek()
	switch {
	case !ok:
	case (t.Kind == Key || t.Kind == Keyword || t.Kind == Variable || t.Kind == Name) && !module(t):
		return p.leaf(NKey)
	case t.Kind == Quote:
		return p.str()
	case t.Kind == Bracket && t.Text == "(":
		return p.paren()
	}
	return p.missing()
}

func (p *parser) value() *Node {
	l := p.binary(1)
	if p.isKind(Pipe) {
		p.next()
		return p.join(NPipe, "", l, p.value())
	}
	return l
}

func (p *parser) ifThen() *Node {
	n := p.opener(NIf)
	n.Kids = append(n.Kids, p.pipe())
	if !p.take(Keyword, "then") {
		return p.unclosed(n)
	}
	n.Kids = append(n.Kids, p.pipe())
	for p.take(Keyword, "elif") {
		n.Kids = append(n.Kids, p.pipe())
		if !p.take(Keyword, "then") {
			return p.unclosed(n)
		}
		n.Kids = append(n.Kids, p.pipe())
	}
	if p.take(Keyword, "else") {
		n.Kids = append(n.Kids, p.pipe())
	}
	if !p.take(Keyword, "end") {
		return p.unclosed(n)
	}
	return p.done(n)
}

// fold is reduce src as pattern (init; update), and foreach's with an extract after.
func (p *parser) fold() *Node {
	kind := NReduce
	if p.is(Keyword, "foreach") {
		kind = NForeach
	}
	n := p.opener(kind)
	n.Kids = append(n.Kids, p.binary(1))
	if !p.take(Keyword, "as") {
		return p.unclosed(n)
	}
	n.Kids = append(n.Kids, p.pattern())
	if !p.is(Bracket, "(") {
		return p.unclosed(n)
	}
	open := p.next()
	n.Kids = append(n.Kids, p.pipe())
	if !p.take(Punct, ";") {
		n.Kids = append(n.Kids, p.missing())
		return p.close(n, open, ")")
	}
	n.Kids = append(n.Kids, p.pipe())
	if kind == NForeach && p.take(Punct, ";") {
		n.Kids = append(n.Kids, p.pipe())
	}
	return p.close(n, open, ")")
}

// defs is def after def, then the query they are defined for: none in a file of defs alone.
func (p *parser) defs() *Node {
	start := p.i
	n := p.open(NDefs, "")
	for p.is(Keyword, "def") {
		d := p.def()
		n.Kids = append(n.Kids, d)
		if d.Open {
			return p.done(n)
		}
	}
	if start == p.lib && p.i == len(p.ts) {
		return p.done(n)
	}
	n.Kids = append(n.Kids, p.pipe())
	return p.done(n)
}

func (p *parser) def() *Node {
	n := p.open(NDef, "")
	p.next()
	if !p.isLocal(Name) {
		return p.unclosed(n)
	}
	n.Text = p.next().Text
	if p.is(Bracket, "(") {
		open := p.next()
		for {
			if p.isLocal(Name) || p.isLocal(Variable) {
				n.Kids = append(n.Kids, p.leaf(NParam))
			} else {
				n.Kids = append(n.Kids, p.missing())
			}
			if !p.take(Punct, ";") {
				break
			}
		}
		if p.close(n, open, ")"); n.Open {
			return n
		}
	}
	if !p.take(Punct, ":") {
		return p.unclosed(n)
	}
	n.Kids = append(n.Kids, p.pipe())
	if !p.take(Punct, ";") {
		return p.unclosed(n)
	}
	return p.done(n)
}

// pattern is what as binds: $x, [p, …], {key: p, $x, …}.
func (p *parser) pattern() *Node {
	t, ok := p.peek()
	switch {
	case !ok:
	case t.Kind == Variable && !module(t):
		return p.leaf(NVar)
	case t.Kind == Bracket && t.Text == "[":
		n := p.open(NPatArray, "")
		open := p.next()
		n.Kids = append(n.Kids, p.pattern())
		for p.take(Punct, ",") {
			n.Kids = append(n.Kids, p.pattern())
		}
		return p.close(n, open, "]")
	case t.Kind == Bracket && t.Text == "{":
		n := p.open(NPatObject, "")
		open := p.next()
		n.Kids = append(n.Kids, p.patEntry())
		for p.take(Punct, ",") {
			n.Kids = append(n.Kids, p.patEntry())
		}
		return p.close(n, open, "}")
	}
	return p.missing()
}

// patEntry is $x, or a key and : and a pattern.
func (p *parser) patEntry() *Node {
	n := p.open(NEntry, "")
	alone := p.isLocal(Variable)
	n.Kids = append(n.Kids, p.key())
	if p.take(Punct, ":") {
		n.Kids = append(n.Kids, p.pattern())
	} else if !alone {
		n.Kids = append(n.Kids, p.missing())
	}
	return p.done(n)
}

// String writes the tree on one line, kind:text(kids…), an Open node's kids ending in …:
// how TestParse's cases are written.
func (n *Node) String() string {
	var b strings.Builder
	n.write(&b)
	return b.String()
}

// spelt are the kinds always written the same way, so String leaves their text out.
var spelt = map[NodeKind]bool{NIdentity: true, NRecurse: true, NOptional: true, NIf: true,
	NTry: true, NReduce: true, NForeach: true}

// Label is the node's kind, and its text when that says more: call:select, binary:>, field:.a.
func (n *Node) Label() string {
	if n.Text != "" && !spelt[n.Kind] {
		return n.Kind.String() + ":" + n.Text
	}
	return n.Kind.String()
}

func (n *Node) write(b *strings.Builder) {
	b.WriteString(n.Label())
	if len(n.Kids) == 0 && !n.Open {
		return
	}
	b.WriteByte('(')
	for i, k := range n.Kids {
		if i > 0 {
			b.WriteByte(' ')
		}
		k.write(b)
	}
	if n.Open {
		if len(n.Kids) > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("…")
	}
	b.WriteByte(')')
}

// Whole is whether nothing in the tree is Open, Missing or Bad: just when gojq parses the text.
func (n *Node) Whole() bool {
	whole := true
	Walk(n, func(x *Node) bool {
		whole = whole && !x.Open && x.Kind != NMissing && x.Kind != NBad && x.Kind != NBroken
		return whole
	})
	return whole
}

// Walk calls f on n and every node under it, parents first; f false skips a node's kids.
func Walk(n *Node, f func(*Node) bool) {
	if n == nil || !f(n) {
		return
	}
	for _, k := range n.Kids {
		Walk(k, f)
	}
}
