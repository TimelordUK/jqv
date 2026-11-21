package jqsyntax

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

func TestParse(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{".", "identity"},
		{"", "defs"},
		{".a | .b", "pipe(field:.a field:.b)"},
		{"1, 2 | 3", "pipe(comma(num:1 num:2) num:3)"},
		{"1 + 2 * 3", "binary:+(num:1 binary:*(num:2 num:3))"},
		{"1 - 2 - 3", "binary:-(binary:-(num:1 num:2) num:3)"},
		{"a // b // c", "binary://(call:a binary://(call:b call:c))"},
		{".a = 1 // 2", "binary://(binary:=(field:.a num:1) num:2)"},
		{".a and .b or .c", "binary:or(binary:and(field:.a field:.b) field:.c)"},
		{`.[0].a[1:]?`, "postfix(identity index(num:0) field:.a slice(num:1 empty) optional)"},
		{`."a b" . "c"`, `postfix(field:.(string(strpart:a b)) field:.(string(strpart:c)))`},
		{`"x\(.a)y"`, "string(strpart:x interp(field:.a) strpart:y)"},
		{"select(.a > 1; 2)", "call:select(binary:>(field:.a num:1) num:2)"},
		{"f (1)", "call:f(num:1)"},
		{"{a, b: 1 | 2, (.c): 3, $x, \"d\": 4}",
			"object(entry(key:a) entry(key:b pipe(num:1 num:2)) entry(paren(field:.c) num:3) entry(key:$x) entry(string(strpart:d) num:4))"},
		{"a, b as $x | c, d", "comma(call:a bind(call:b var:$x comma(call:c call:d)))"},
		{". as [$a, {b: $c, $d}] ?// $e | 1", "bind(identity patarray(var:$a patobject(entry(key:b var:$c) entry(key:$d))) var:$e num:1)"},
		{"reduce .[] as $x (0; . + $x)", "reduce(postfix(identity iterate) var:$x num:0 binary:+(identity var:$x))"},
		{"foreach .[] as $x (0; 1; 2)", "foreach(postfix(identity iterate) var:$x num:0 num:1 num:2)"},
		{"if . then 1 elif 2 then 3 else 4 end", "if(identity num:1 num:2 num:3 num:4)"},
		{"try .a catch . | 1", "pipe(try(field:.a identity) num:1)"},
		{"try .a + 1", "binary:+(try(field:.a) num:1)"},
		{"-.a[0]", "unary:-(postfix(field:.a index(num:0)))"},
		{"def f(g; $x): g; f(.)", "defs(def:f(param:g param:$x call:g) call:f(identity))"},
		{"def f: 1; def g: 2;", "defs(def:f(num:1) def:g(num:2))"},
		{"1, def f: 2; f | 3", "comma(num:1 defs(def:f(num:2) pipe(call:f num:3)))"},
		{"label $out | 1, break $out", "label:$out(comma(num:1 break:$out))"},
		{`@base64 "x"`, "format:@base64(string(strpart:x))"},
		{`import "a" as b; include "c" {x: 1}; .`,
			`module(directive:import(string(strpart:a) key:b) directive:include(string(strpart:c) object(entry(key:x num:1))) identity)`},
		// half typed
		{".a | ", "pipe(field:.a missing)"},
		{"select(.a | len", "call:select(pipe(field:.a call:len) …)"},
		{"{a: 1, b", "object(entry(key:a num:1) entry(key:b) …)"},
		{`"x\(.a`, "string(strpart:x interp(field:.a …) …)"},
		{"if . then 1", "if(identity num:1 …)"},
		{". as $x", "bind(identity var:$x …)"},
		{"def f: 1", "defs(def:f(num:1 …))"},
		{"(def f: 1;", "paren(defs(def:f(num:1) missing) …)"},
		{"reduce . as $x (0)", "reduce(identity var:$x num:0 missing)"},
		// mistakes
		{"(1 2)", "paren(broken(num:1 bad:2))"},
		{".a )", "broken(field:.a bad:))"},
		{".a .b foo", "broken(postfix(field:.a field:.b) bad:foo)"},
	} {
		if got := Parse(c.in).String(); got != c.want {
			t.Errorf("Parse(%q)\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}

func TestParseSpans(t *testing.T) {
	in := `.a | select(.b > 1) | {c: "x\(.d)"}`
	got := map[string]string{}
	Walk(Parse(in), func(n *Node) bool {
		got[n.Kind.String()+":"+n.Text] = string([]rune(in)[n.Pos:n.End])
		return true
	})
	for k, want := range map[string]string{
		"call:select": "select(.b > 1)",
		"binary:>":    ".b > 1",
		"object:":     `{c: "x\(.d)"}`,
		"string:":     `"x\(.d)"`,
		"interp:":     `\(.d)`,
		"field:.d":    ".d",
	} {
		if got[k] != want {
			t.Errorf("%s spans %q, want %q", k, got[k], want)
		}
	}
}

// TestParseWhole holds Whole to gojq on the places a parser is easily too lenient: def and
// label only start a query, = and == don't chain, module comes first.
func TestParseWhole(t *testing.T) {
	for _, s := range []string{"1 + def f: 1; f", "1 + label $x | 2", "try def f: 1; f", "-def f: 1; f",
		"{a: def f: 1; f}", "reduce def f: 1; f as $x (0; 1)", "def f: 1; f as $x | 2", "1 as $x | 2 as $y | 3",
		"f()", "{(1)}", "{a: 1, }", ".[]?", "..[]", "..a", "$__loc__", "{$__loc__}", "@base64d", "1 as [$a, $b] | 1",
		"{if: 1}", "{a: .b as $x | $x}", `import "a" as $b; .`, "def f($a; g): 1; 2", "if . then 1 end",
		". as {a: $x} ?// [$x] | 1", "def if: 1; 2", "{true}", `{"a\(1)"}`, ".a.[0]", `.["a"]`,
		"try (1) catch (2) | 3", "1 ? // 2", ".a?//1", "label $f | label $g | 1", "reduce . as [$a] (.; .)",
		"1 as $x | 2, 3 as $y | 4", `include "a"; module {};`, "module {a: [1, {b: null}]}; 1",
		"0=0=0", "1 < 2 == 3", "1 == 2 and 3 == 4", ".a = .b | .c = 1",
	} {
		_, err := gojq.Parse(s)
		if root := Parse(s); root.Whole() != (err == nil) {
			t.Errorf("Parse(%q) = %s, whole %v; gojq: %v", s, root, root.Whole(), err)
		}
	}
}

// TestParseExamples parses the programs under examples/jq: many lines, comments and defs.
func TestParseExamples(t *testing.T) {
	files, err := filepath.Glob("../../examples/jq/*/query.jq")
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parseInvariants(t, string(b))
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"[.[].people[] | select(.age > 30) | .age] | add",
		`{a: "x\(.b | "y\(.c)")", "k": [1, 2.5e3]} | .a //= $__loc__`,
		"reduce .[] as {a: $x} (0; . + $x) | @csv",
		"def f(g): g | g; f(.a?) as [$a] ?// $b | .. | try error catch .",
		"# c \\\n c\n.a | a::b", `"é\\\"\/"`, ".[1:]", ". [0]", `. "a"`, "1 .a",
		"if . then 1 elif 2 then 3 else 4 end", "if . then 1 end",
		"label $out | foreach .[] as $i (0; .+$i; if . > 3 then ., break $out else . end)",
		`module {a: [1, "x", null]}; import "a" as $b; include "c"; def f: 1;`,
		"{if: 1, $x, (1): 2, \"a\\(1)\": 3, a: 1 | 2,}", "-1 - -.a", "@json \"x\\(.)\"",
		". as {$a: [$b], \"c\": $d, (1): $e} | 1", "a, b as $x | c, d", "1, def f: 2; f | 3",
	} {
		f.Add(s)
	}
	for _, k := range []string{"or", "and", "module", "import", "include", "def", "as", "label",
		"break", "null", "true", "false", "if", "then", "elif", "else", "end", "try", "catch",
		"reduce", "foreach"} {
		f.Add(k)
		f.Add(".a " + k)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return
		}
		parseInvariants(t, s)
		recursiveInvariants(t, s)
	})
}

// recursiveInvariants: what Recursive finds (Q-052) is inside the text, and each def's name
// is its own name.
func recursiveInvariants(t *testing.T, s string) {
	t.Helper()
	rs := []rune(s)
	in := func(sp [2]int) bool { return 0 <= sp[0] && sp[0] <= sp[1] && sp[1] <= len(rs) }
	r := Recursive(s)
	for _, d := range r.Defs {
		if !in(d.Name) || string(rs[d.Name[0]:d.Name[1]]) != d.Def.Text {
			t.Fatalf("Recursive(%q): %s named at %v", s, d.Def.Text, d.Name)
		}
		for _, c := range d.Calls {
			if !in(c) {
				t.Fatalf("Recursive(%q): a call to %s at %v", s, d.Def.Text, c)
			}
		}
	}
	for _, b := range r.Builtins {
		if !in(b) {
			t.Fatalf("Recursive(%q): a builtin at %v", s, b)
		}
	}
}

// parseInvariants: the spans nest and keep order; the tree is whole just when gojq parses
// the text, and then prints as gojq's; on text gojq finds unfinished, something in it is Open
// or Missing.
func parseInvariants(t *testing.T, s string) {
	t.Helper()
	root := Parse(s)
	n := len([]rune(s))
	var spans func(*Node)
	spans = func(x *Node) {
		if x.Pos < 0 || x.End < x.Pos || x.End > n {
			t.Fatalf("Parse(%q): %s spans %d..%d", s, x.Kind, x.Pos, x.End)
		}
		end := x.Pos
		for _, k := range x.Kids {
			if k.Pos < end || k.End > x.End {
				t.Fatalf("Parse(%q): %s %d..%d out of its %s %d..%d (after %d)\n%s",
					s, k.Kind, k.Pos, k.End, x.Kind, x.Pos, x.End, end, root)
			}
			end = k.End
			spans(k)
		}
	}
	spans(root)
	q, err := gojq.Parse(s)
	if err != nil {
		if root.Whole() {
			t.Fatalf("Parse(%q) = %s: whole, but gojq: %v", s, root, err)
		}
		open := false
		Walk(root, func(x *Node) bool {
			open = open || x.Open || x.Kind == NMissing
			return true
		})
		if strings.Contains(err.Error(), "unexpected EOF") && !open {
			t.Fatalf("Parse(%q) = %s: nothing open, but gojq: %v", s, root, err)
		}
		return
	}
	if !root.Whole() {
		t.Fatalf("Parse(%q) = %s: not whole, but gojq parses it", s, root)
	}
	want := q.String()
	srcOf = s
	got, cerr := toGojq(root)
	if cerr != nil {
		t.Fatalf("Parse(%q) = %s: %v", s, root, cerr)
	}
	if got.String() != want {
		t.Fatalf("Parse(%q) = %s\n prints %s\ngojq's %s", s, root, got, want)
	}
}

// toGojq makes gojq's tree of a whole one of ours, so the two print alike.
func toGojq(n *Node) (q *gojq.Query, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return gq(n), nil
}

var gojqOps = map[string]gojq.Operator{"|": gojq.OpPipe, ",": gojq.OpComma, "+": gojq.OpAdd,
	"-": gojq.OpSub, "*": gojq.OpMul, "/": gojq.OpDiv, "%": gojq.OpMod, "==": gojq.OpEq,
	"!=": gojq.OpNe, ">": gojq.OpGt, "<": gojq.OpLt, ">=": gojq.OpGe, "<=": gojq.OpLe,
	"and": gojq.OpAnd, "or": gojq.OpOr, "//": gojq.OpAlt, "=": gojq.OpAssign, "|=": gojq.OpModify,
	"+=": gojq.OpUpdateAdd, "-=": gojq.OpUpdateSub, "*=": gojq.OpUpdateMul, "/=": gojq.OpUpdateDiv,
	"%=": gojq.OpUpdateMod, "//=": gojq.OpUpdateAlt}

func gq(n *Node) *gojq.Query {
	k := n.Kids
	switch n.Kind {
	case NPipe:
		return &gojq.Query{Left: gq(k[0]), Op: gojq.OpPipe, Right: gq(k[1])}
	case NComma:
		return &gojq.Query{Left: gq(k[0]), Op: gojq.OpComma, Right: gq(k[1])}
	case NBinary:
		return &gojq.Query{Left: gq(k[0]), Op: gojqOps[n.Text], Right: gq(k[1])}
	case NBind:
		q := &gojq.Query{Left: gq(k[0]), Op: gojq.OpPipe, Right: gq(k[len(k)-1])}
		for _, p := range k[1 : len(k)-1] {
			q.Patterns = append(q.Patterns, gpat(p))
		}
		return q
	case NDefs:
		q := &gojq.Query{}
		var fds []*gojq.FuncDef
		for _, d := range k {
			if d.Kind != NDef {
				q = gq(d)
				continue
			}
			fd := &gojq.FuncDef{Name: d.Text, Body: gq(d.Kids[len(d.Kids)-1])}
			for _, a := range d.Kids[:len(d.Kids)-1] {
				fd.Args = append(fd.Args, a.Text)
			}
			fds = append(fds, fd)
		}
		q.FuncDefs = append(fds, q.FuncDefs...)
		return q
	case NModule:
		q := &gojq.Query{}
		if last := k[len(k)-1]; last.Kind != NDirective {
			q = gq(last)
			k = k[:len(k)-1]
		}
		for _, d := range k {
			var meta *gojq.ConstObject
			if m := metaOf(d); m != "" {
				mq, err := gojq.Parse("module " + m + ";")
				if err != nil {
					panic(err)
				}
				meta = mq.Meta
			}
			switch d.Text {
			case "module":
				q.Meta = meta
			case "import":
				q.Imports = append(q.Imports, &gojq.Import{ImportPath: gstr(d.Kids[0]).Str, ImportAlias: d.Kids[1].Text, Meta: meta})
			case "include":
				q.Imports = append(q.Imports, &gojq.Import{IncludePath: gstr(d.Kids[0]).Str, Meta: meta})
			}
		}
		return q
	}
	return &gojq.Query{Term: gterm(n)}
}

// srcOf is the text being converted: a directive's meta is a constant object, which gojq
// reads for us from its text.
var srcOf string

func metaOf(d *Node) string {
	for _, x := range d.Kids {
		if x.Kind == NObject {
			return string([]rune(srcOf)[x.Pos:x.End])
		}
	}
	return ""
}

func gterm(n *Node) *gojq.Term {
	k := n.Kids
	switch n.Kind {
	case NIdentity:
		return &gojq.Term{Type: gojq.TermTypeIdentity}
	case NRecurse:
		return &gojq.Term{Type: gojq.TermTypeRecurse}
	case NLiteral:
		return &gojq.Term{Type: map[string]gojq.TermType{"null": gojq.TermTypeNull,
			"true": gojq.TermTypeTrue, "false": gojq.TermTypeFalse}[n.Text]}
	case NNumber:
		return &gojq.Term{Type: gojq.TermTypeNumber, Number: n.Text}
	case NField:
		return &gojq.Term{Type: gojq.TermTypeIndex, Index: gindex(n)}
	case NString:
		return &gojq.Term{Type: gojq.TermTypeString, Str: gstr(n)}
	case NFormat:
		t := &gojq.Term{Type: gojq.TermTypeFormat, Format: n.Text}
		if len(k) > 0 {
			t.Str = gstr(k[0])
		}
		return t
	case NVar:
		return &gojq.Term{Type: gojq.TermTypeFunc, Func: &gojq.Func{Name: n.Text}}
	case NCall:
		f := &gojq.Func{Name: n.Text}
		for _, a := range k {
			f.Args = append(f.Args, gq(a))
		}
		return &gojq.Term{Type: gojq.TermTypeFunc, Func: f}
	case NParen:
		return &gojq.Term{Type: gojq.TermTypeQuery, Query: gq(k[0])}
	case NArray:
		a := &gojq.Array{}
		if len(k) > 0 {
			a.Query = gq(k[0])
		}
		return &gojq.Term{Type: gojq.TermTypeArray, Array: a}
	case NObject:
		o := &gojq.Object{}
		for _, e := range k {
			kv := &gojq.ObjectKeyVal{}
			switch key := e.Kids[0]; key.Kind {
			case NKey:
				kv.Key = key.Text
			case NString:
				kv.KeyString = gstr(key)
			case NParen:
				kv.KeyQuery = gq(key.Kids[0])
			}
			if len(e.Kids) > 1 {
				kv.Val = gq(e.Kids[1])
			}
			o.KeyVals = append(o.KeyVals, kv)
		}
		return &gojq.Term{Type: gojq.TermTypeObject, Object: o}
	case NUnary:
		op := gojq.OpAdd
		if n.Text == "-" {
			op = gojq.OpSub
		}
		return &gojq.Term{Type: gojq.TermTypeUnary, Unary: &gojq.Unary{Op: op, Term: gterm(k[0])}}
	case NIf:
		f := &gojq.If{Cond: gq(k[0]), Then: gq(k[1])}
		i := 2
		for ; i+1 < len(k); i += 2 {
			f.Elif = append(f.Elif, &gojq.IfElif{Cond: gq(k[i]), Then: gq(k[i+1])})
		}
		if i < len(k) {
			f.Else = gq(k[i])
		}
		return &gojq.Term{Type: gojq.TermTypeIf, If: f}
	case NTry:
		t := &gojq.Try{Body: gq(k[0])}
		if len(k) > 1 {
			t.Catch = gq(k[1])
		}
		return &gojq.Term{Type: gojq.TermTypeTry, Try: t}
	case NReduce:
		return &gojq.Term{Type: gojq.TermTypeReduce, Reduce: &gojq.Reduce{Query: gq(k[0]),
			Pattern: gpat(k[1]), Start: gq(k[2]), Update: gq(k[3])}}
	case NForeach:
		f := &gojq.Foreach{Query: gq(k[0]), Pattern: gpat(k[1]), Start: gq(k[2]), Update: gq(k[3])}
		if len(k) > 4 {
			f.Extract = gq(k[4])
		}
		return &gojq.Term{Type: gojq.TermTypeForeach, Foreach: f}
	case NLabel:
		return &gojq.Term{Type: gojq.TermTypeLabel, Label: &gojq.Label{Ident: n.Text, Body: gq(k[0])}}
	case NBreak:
		return &gojq.Term{Type: gojq.TermTypeBreak, Break: n.Text}
	case NPostfix:
		t := gterm(k[0])
		for _, s := range k[1:] {
			switch s.Kind {
			case NField:
				t.SuffixList = append(t.SuffixList, &gojq.Suffix{Index: gindex(s)})
			case NIndex:
				t.SuffixList = append(t.SuffixList, &gojq.Suffix{Index: &gojq.Index{Start: gq(s.Kids[0])}})
			case NSlice:
				x := &gojq.Index{IsSlice: true}
				if s.Kids[0].Kind != NEmpty {
					x.Start = gq(s.Kids[0])
				}
				if s.Kids[1].Kind != NEmpty {
					x.End = gq(s.Kids[1])
				}
				t.SuffixList = append(t.SuffixList, &gojq.Suffix{Index: x})
			case NIterate:
				t.SuffixList = append(t.SuffixList, &gojq.Suffix{Iter: true})
			case NOptional:
				t.SuffixList = append(t.SuffixList, &gojq.Suffix{Optional: true})
			default:
				panic("suffix " + s.Kind.String())
			}
		}
		return t
	case NDefs, NPipe, NComma, NBinary, NBind:
		return &gojq.Term{Type: gojq.TermTypeQuery, Query: gq(n)}
	}
	panic("term " + n.Kind.String())
}

func gindex(n *Node) *gojq.Index {
	if n.Text == "." {
		return &gojq.Index{Str: gstr(n.Kids[0])}
	}
	return &gojq.Index{Name: n.Text[1:]}
}

func gpat(n *Node) *gojq.Pattern {
	switch n.Kind {
	case NVar:
		return &gojq.Pattern{Name: n.Text}
	case NPatArray:
		p := &gojq.Pattern{}
		for _, k := range n.Kids {
			p.Array = append(p.Array, gpat(k))
		}
		return p
	case NPatObject:
		p := &gojq.Pattern{}
		for _, e := range n.Kids {
			po := &gojq.PatternObject{}
			switch key := e.Kids[0]; key.Kind {
			case NKey:
				po.Key = key.Text
			case NString:
				po.KeyString = gstr(key)
			case NParen:
				po.KeyQuery = gq(key.Kids[0])
			}
			if len(e.Kids) > 1 {
				po.Val = gpat(e.Kids[1])
			}
			p.Object = append(p.Object, po)
		}
		return p
	}
	panic("pattern " + n.Kind.String())
}

// gstr decodes a string as gojq's lexer does: its text through JSON, control characters escaped.
func gstr(n *Node) *gojq.String {
	decode := func(raw string) string {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range raw {
			if r < ' ' {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		var s string
		if err := json.Unmarshal([]byte(b.String()), &s); err != nil {
			panic(err)
		}
		return s
	}
	interp := false
	for _, k := range n.Kids {
		interp = interp || k.Kind == NInterp
	}
	if !interp {
		raw := ""
		for _, k := range n.Kids {
			raw += k.Text
		}
		return &gojq.String{Str: decode(raw)}
	}
	s := &gojq.String{Queries: []*gojq.Query{}}
	for _, k := range n.Kids {
		if k.Kind == NStrPart {
			s.Queries = append(s.Queries, &gojq.Query{Term: &gojq.Term{Type: gojq.TermTypeString, Str: &gojq.String{Str: decode(k.Text)}}})
		} else {
			s.Queries = append(s.Queries, &gojq.Query{Term: &gojq.Term{Type: gojq.TermTypeQuery, Query: gq(k.Kids[0])}})
		}
	}
	return s
}
