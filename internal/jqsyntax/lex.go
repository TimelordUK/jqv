package jqsyntax

// Kind is what a Token is (DESIGN §19).
type Kind int

const (
	Space    Kind = iota // spaces, tabs, newlines
	Comment              // # to the end of the line, a \ at the end carrying it on
	Quote                // a string's opening or closing "
	Text                 // a run of a string's plain characters
	Escape               // \n, \", é in a string
	Interp               // a string's \( and the ) that ends it
	Number               // 1, 1.5, .5, 1e3
	Field                // .foo, and the . of ."foo"
	Dot                  // .
	Recurse              // ..
	Variable             // $x, $__loc__, $mod::x
	Format               // @csv
	Keyword              // if, def, reduce, and, or, true, null, …
	Name                 // a call or a def's name: select, mod::f
	Key                  // an object's key written as a name: {a: 1}, {if: 1}, {a}
	Pipe                 // |
	Operator             // + - * / % == != < <= > >= // ?// ? = |= += -= *= /= %= //=
	Punct                // , ; :
	Bracket              // ( ) [ ] { }
	Error                // what gojq won't read: 1a, \q in a string, !, ^, a lone $, a stray )
)

var kindNames = [...]string{"space", "comment", "quote", "text", "escape", "interp", "num", "field",
	"dot", "recurse", "var", "format", "kw", "name", "key", "pipe", "op", "punct", "bracket", "error"}

func (k Kind) String() string { return kindNames[k] }

// Token is a piece of jq text, Pos to End in runes. Depth is how many brackets are open around
// it, a string's \( counted: a bracket and its closer have the same depth, the one outside
// what they hold, so rainbow colour is by Depth. A closer that matches nothing open is an Error.
type Token struct {
	Kind     Kind
	Pos, End int
	Depth    int
	Text     string
}

// keywords are gojq's (lexer.go).
var keywords = map[string]bool{
	"or": true, "and": true, "module": true, "import": true, "include": true, "def": true,
	"as": true, "label": true, "break": true, "null": true, "true": true, "false": true,
	"if": true, "then": true, "elif": true, "else": true, "end": true, "try": true,
	"catch": true, "reduce": true, "foreach": true,
}

// Lex reads all of the text into tokens: joined, they give the text back, and it never fails,
// since the line is nearly always half typed. An unterminated string runs to the end; what gojq
// wouldn't read is an Error token. The rules are gojq's (names are ASCII, numbers as gojq reads
// them), so text gojq parses has no Error tokens.
func Lex(text string) []Token {
	rs := []rune(text)
	var ts []Token
	// stack holds the open brackets, '(' '[' '{', and 'i' for a string's \(.
	var stack []rune
	add := func(k Kind, i, j int) { ts = append(ts, Token{k, i, j, len(stack), string(rs[i:j])}) }
	peek := func(i int) rune {
		if i < len(rs) {
			return rs[i]
		}
		return 0
	}
	top := func() rune {
		if len(stack) == 0 {
			return 0
		}
		return stack[len(stack)-1]
	}
	// str reads a string from i, inside it, to its closing quote or a \(; it gives where code resumes.
	str := func(i int) int {
		start := i
		text := func() {
			if i > start {
				add(Text, start, i)
			}
		}
		for i < len(rs) {
			switch rs[i] {
			case '"':
				text()
				add(Quote, i, i+1)
				return i + 1
			case '\\':
				text()
				n := 2
				switch peek(i + 1) {
				case '(':
					add(Interp, i, i+2)
					stack = append(stack, 'i')
					return i + 2
				case '"', '/', '\\', 'b', 'f', 'n', 'r', 't':
					add(Escape, i, i+2)
				case 'u':
					for n < 6 && isHex(peek(i+n)) {
						n++
					}
					if n == 6 {
						add(Escape, i, i+n)
					} else {
						add(Error, i, i+n)
					}
				case 0:
					n = 1
					add(Error, i, i+1)
				default:
					add(Error, i, i+2)
				}
				i += n
				start = i
				continue
			}
			i++
		}
		text()
		return i
	}
	// key is whether a name at this point is an object's key: just inside { or after its comma.
	key := func() bool {
		if top() != '{' {
			return false
		}
		for j := len(ts) - 1; j >= 0; j-- {
			switch ts[j].Kind {
			case Space, Comment:
				continue
			case Bracket:
				return ts[j].Text == "{"
			case Punct:
				return ts[j].Text == ","
			}
			return false
		}
		return false
	}
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case isWhite(r):
			j := i
			for j < len(rs) && isWhite(rs[j]) {
				j++
			}
			add(Space, i, j)
			i = j
		case r == '#':
			j := i + 1
			for j < len(rs) && rs[j] != '\n' && rs[j] != '\r' {
				if rs[j] == '\\' { // gojq: a \ carries the comment over a newline, \\ is a \
					j++
					switch peek(j) {
					case '\\', '\n':
						j++
					case '\r':
						j++
						if peek(j) == '\n' {
							j++
						}
					}
					continue
				}
				j++
			}
			add(Comment, i, j)
			i = j
		case r == '"':
			add(Quote, i, i+1)
			i = str(i + 1)
		case isIdent(r, false):
			j := ident(rs, i)
			k := Name
			switch {
			case key():
				k = Key
			case keywords[string(rs[i:j])]:
				k = Keyword
			}
			add(k, i, j)
			i = j
		case isDigit(r):
			j, ok := number(rs, i+1, lead)
			add(numKind(ok), i, j)
			i = j
		case r == '.':
			switch n := peek(i + 1); {
			case n == '.':
				add(Recurse, i, i+2)
				i += 2
			case isIdent(n, false):
				j := i + 1
				for j < len(rs) && isIdent(rs[j], true) {
					j++
				}
				add(Field, i, j)
				i = j
			case isDigit(n):
				j, ok := number(rs, i+1, float)
				add(numKind(ok), i, j)
				i = j
			case n == '"':
				add(Field, i, i+1)
				i++
			default:
				add(Dot, i, i+1)
				i++
			}
		case r == '$':
			if isIdent(peek(i+1), false) {
				j := ident(rs, i+1)
				add(Variable, i, j)
				i = j
			} else {
				add(Error, i, i+1)
				i++
			}
		case r == '@':
			if isIdent(peek(i+1), true) {
				j := i + 1
				for j < len(rs) && isIdent(rs[j], true) {
					j++
				}
				add(Format, i, j)
				i = j
			} else {
				add(Error, i, i+1)
				i++
			}
		case r == '(' || r == '[' || r == '{':
			add(Bracket, i, i+1)
			stack = append(stack, r)
			i++
		case r == ')' && top() == 'i':
			stack = stack[:len(stack)-1]
			add(Interp, i, i+1)
			i = str(i + 1)
		case r == ')' || r == ']' || r == '}':
			if top() == map[rune]rune{')': '(', ']': '[', '}': '{'}[r] {
				stack = stack[:len(stack)-1]
				add(Bracket, i, i+1)
			} else { // closes nothing, or not what is open: left open, for the closer to come
				add(Error, i, i+1)
			}
			i++
		case r == ',' || r == ';' || r == ':':
			add(Punct, i, i+1)
			i++
		default:
			if n := operator(rs, i); n > 0 {
				k := Operator
				if n == 1 && r == '|' {
					k = Pipe
				}
				add(k, i, i+n)
				i += n
			} else {
				add(Error, i, i+1)
				i++
			}
		}
	}
	return ts
}

// operator is the length of the operator at i, longest first, or 0.
func operator(rs []rune, i int) int {
	for _, op := range []string{"?//", "//=", "|=", "+=", "-=", "*=", "/=", "%=", "==", "!=", "<=", ">=", "//",
		"|", "+", "-", "*", "/", "%", "<", ">", "=", "?"} {
		if hasAt(rs, i, op) {
			return len(op)
		}
	}
	return 0
}

func hasAt(rs []rune, i int, s string) bool {
	for _, c := range s {
		if i >= len(rs) || rs[i] != c {
			return false
		}
		i++
	}
	return true
}

// ident reads a name from i, and a module's ::name after it, as gojq's scanIdentOrModule does.
func ident(rs []rune, i int) int {
	for i < len(rs) && isIdent(rs[i], true) {
		i++
	}
	if i+2 < len(rs) && rs[i] == ':' && rs[i+1] == ':' && isIdent(rs[i+2], false) {
		i += 2
		for i < len(rs) && isIdent(rs[i], true) {
			i++
		}
	}
	return i
}

const (
	lead = iota
	float
	expLead
	exp
)

// number reads on from i in the given state, as gojq's scanNumber does: where it ends, and
// whether gojq takes it (1a, 1.2.3 and 1e aren't numbers; the Error runs as far as gojq's).
func number(rs []rune, i, state int) (int, bool) {
	at := func(i int) rune {
		if i < len(rs) {
			return rs[i]
		}
		return 0
	}
	for {
		ch := at(i)
		switch state {
		case lead, float:
			switch {
			case isDigit(ch):
				i++
			case ch == '.':
				i++
				if state != lead {
					return i, false
				}
				state = float
			case ch == 'e' || ch == 'E':
				i++
				if c := at(i); c == '-' || c == '+' {
					i++
				}
				state = expLead
			case isIdent(ch, false):
				return i + 1, false
			default:
				return i, true
			}
		default:
			switch {
			case isDigit(ch):
				i++
				state = exp
			case isIdent(ch, false):
				return i + 1, false
			default:
				return i, state == exp
			}
		}
	}
}

func isWhite(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

func isIdent(r rune, tail bool) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_' || tail && isDigit(r)
}

func isDigit(r rune) bool { return '0' <= r && r <= '9' }

func isHex(r rune) bool { return isDigit(r) || 'a' <= r && r <= 'f' || 'A' <= r && r <= 'F' }

func numKind(ok bool) Kind {
	if ok {
		return Number
	}
	return Error
}
