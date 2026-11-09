package jqsyntax

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

// marks writes tokens as [kind:text], spaces left as they are, so a case reads.
func marks(ts []Token) string {
	var b strings.Builder
	for _, t := range ts {
		if t.Kind == Space {
			b.WriteString(t.Text)
			continue
		}
		fmt.Fprintf(&b, "[%s:%s]", t.Kind, t.Text)
	}
	return b.String()
}

func TestLex(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"select(.age > 30)", "[name:select][bracket:(][field:.age] [op:>] [num:30][bracket:)]"},
		{".select | .end", "[field:.select] [pipe:|] [field:.end]"},
		{"{if: 1, a, b: .c}", "[bracket:{][key:if][punct::] [num:1][punct:,] [key:a][punct:,] [key:b][punct::] [field:.c][bracket:}]"},
		{"{a: if . then 1 else 2 end}", "[bracket:{][key:a][punct::] [kw:if] [dot:.] [kw:then] [num:1] [kw:else] [num:2] [kw:end][bracket:}]"},
		{"$__loc__", "[var:$__loc__]"},
		{`@base64d "a\(.x)b"`, "[format:@base64d] [quote:\"][text:a][interp:\\(][field:.x][interp:)][text:b][quote:\"]"},
		{`"a\nb\u00e9\(1)"`, `[quote:"][text:a][escape:\n][text:b][escape:\u00e9][interp:\(][num:1][interp:)][quote:"]`},
		{`"x\(")")y"`, `[quote:"][text:x][interp:\(][quote:"][text:)][quote:"][interp:)][text:y][quote:"]`},
		{".a ?// .b", "[field:.a] [op:?//] [field:.b]"},
		{".a //= 1", "[field:.a] [op://=] [num:1]"},
		{".a |= . + 1", "[field:.a] [op:|=] [dot:.] [op:+] [num:1]"},
		{".[0]", "[dot:.][bracket:[][num:0][bracket:]]"},
		{"..", "[recurse:..]"},
		{"..|.a?", "[recurse:..][pipe:|][field:.a][op:?]"},
		{`."a b"`, `[field:.][quote:"][text:a b][quote:"]`},
		{"1e3, 1.5, .5, 1., 1E-2", "[num:1e3][punct:,] [num:1.5][punct:,] [num:.5][punct:,] [num:1.][punct:,] [num:1E-2]"},
		{"a::b($m::x)", "[name:a::b][bracket:(][var:$m::x][bracket:)]"},
		{"def f: 1; f", "[kw:def] [name:f][punct::] [num:1][punct:;] [name:f]"},
		{"reduce .[] as $x (0; . + $x)", "[kw:reduce] [dot:.][bracket:[][bracket:]] [kw:as] [var:$x] [bracket:(][num:0][punct:;] [dot:.] [op:+] [var:$x][bracket:)]"},
		{".a and .b or (.c|not)", "[field:.a] [kw:and] [field:.b] [kw:or] [bracket:(][field:.c][pipe:|][name:not][bracket:)]"},
		{".a # x | y\n| .b", "[field:.a] [comment:# x | y]\n[pipe:|] [field:.b]"},
		{"1 # a \\\n still comment\n| .b", "[num:1] [comment:# a \\\n still comment]\n[pipe:|] [field:.b]"},
		// half typed
		{`"abc`, `[quote:"][text:abc]`},
		{`"a\(.b`, `[quote:"][text:a][interp:\(][field:.b]`},
		{`"a\`, `[quote:"][text:a][error:\]`},
		{"[.a | ", "[bracket:[][field:.a] [pipe:|] "},
		// errors
		{"1a", "[error:1a]"},
		{"1e", "[error:1e]"},
		{"1.2.3", "[error:1.2.][num:3]"},
		{`"\q"`, `[quote:"][error:\q][quote:"]`},
		{`"\u12g"`, `[quote:"][error:\u12][text:g][quote:"]`},
		{".a ! $", "[field:.a] [error:!] [error:$]"},
		{".café", "[field:.caf][error:é]"},
		{"[)]", "[bracket:[][error:)][bracket:]]"},
	} {
		if got := marks(Lex(c.in)); got != c.want {
			t.Errorf("Lex(%q)\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

func TestLexDepth(t *testing.T) {
	var b strings.Builder
	for _, t := range Lex(`[.a, {b: (1)}, "\(.c)"]`) {
		if t.Kind != Space {
			fmt.Fprintf(&b, "%s%d ", t.Text, t.Depth)
		}
	}
	want := `[0 .a1 ,1 {1 b2 :2 (2 13 )2 }1 ,1 "1 \(1 .c2 )1 "1 ]0 `
	if b.String() != want {
		t.Errorf("depths\n got %s\nwant %s", b.String(), want)
	}
}

// lexInvariants: the tokens cover the text, in order, none empty; for text gojq parses, no
// Error, and each bracket's closer at its depth.
func lexInvariants(t *testing.T, s string) {
	ts := Lex(s)
	var b strings.Builder
	at := 0
	for _, tk := range ts {
		if tk.Pos != at || tk.End <= tk.Pos || tk.Text != string([]rune(s)[tk.Pos:tk.End]) {
			t.Fatalf("Lex(%q): token %+v at %d", s, tk, at)
		}
		at = tk.End
		b.WriteString(tk.Text)
	}
	if b.String() != s {
		t.Fatalf("Lex(%q) joined gives %q", s, b.String())
	}
	if _, err := gojq.Parse(s); err != nil {
		return
	}
	var open []Token
	for _, tk := range ts {
		switch {
		case tk.Kind == Error:
			t.Fatalf("Lex(%q): gojq parses it, but %+v", s, tk)
		case (tk.Kind == Bracket || tk.Kind == Interp) && strings.ContainsAny(tk.Text, "([{"):
			open = append(open, tk)
		case tk.Kind == Bracket || tk.Kind == Interp:
			o := open[len(open)-1]
			open = open[:len(open)-1]
			if o.Depth != tk.Depth || o.Depth != len(open) {
				t.Fatalf("Lex(%q): %+v closes %+v", s, tk, o)
			}
		}
	}
	if len(open) > 0 {
		t.Fatalf("Lex(%q): %+v not closed", s, open)
	}
}

func FuzzLex(f *testing.F) {
	for _, s := range []string{
		"[.[].people[] | select(.age > 30) | .age] | add",
		`{a: "x\(.b | "y\(.c)")", "k": [1, 2.5e3]} | .a //= $__loc__`,
		"reduce .[] as {a: $x} (0; . + $x) | @csv",
		"def f(g): g | g; f(.a?) ?// [.b] | .. | try error catch .",
		"# c \\\n c\n.a | a::b",
		`"é\\\"\/"`, "1.2.3", ".[1:]", "if . then 1 elif 2 then 3 else 4 end",
		"label $out | foreach .[] as $i (0; .+$i; if . > 3 then ., break $out else . end)",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// invalid UTF-8 doesn't survive []rune; gojq takes a NUL as the end of the text (its
		// lexer's peek gives 0 there), where we call it an Error, as jq does
		if utf8.ValidString(s) && !strings.ContainsRune(s, 0) {
			lexInvariants(t, s)
		}
	})
}
