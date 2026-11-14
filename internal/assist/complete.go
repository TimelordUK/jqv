package assist

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/catalog"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/itchyny/gojq"
)

// Complete is Tab's completer (Q-036, DESIGN §18): the : command being typed, a :help topic,
// or on a line of jq the name or $variable under the cursor. Inside a string, a comment or a
// .field it offers nothing (fields are for keys, R-006). The context's defs (Q-037) will be
// one more source.
func Complete(line string, cur int) (from, to int, cands []editor.Candidate) {
	rs := []rune(line)
	cur = min(max(cur, 0), len(rs))
	lead := 0
	for lead < len(rs) && unicode.IsSpace(rs[lead]) {
		lead++
	}
	if lead < len(rs) && rs[lead] == ':' {
		end := lead + 1
		for end < len(rs) && !unicode.IsSpace(rs[end]) {
			end++
		}
		if cur <= lead { // before the colon: nothing to complete
			return cur, cur, nil
		}
		if cur <= end {
			return lead, end, commands(string(rs[lead+1 : cur]))
		}
		if name := string(rs[lead+1 : end]); name == "help" || name == "h" || name == "?" {
			arg := end
			for arg < len(rs) && unicode.IsSpace(rs[arg]) {
				arg++
			}
			if cur < arg {
				return cur, cur, nil
			}
			return arg, len(rs), topics(string(rs[arg:cur]))
		}
	}
	off, expr, ok := jqPart(line)
	if !ok {
		return cur, cur, nil
	}
	from, to, cands = jqWord(expr, cur-off)
	return off + from, off + to, cands
}

func commands(prefix string) []editor.Candidate {
	var cs []editor.Candidate
	for _, c := range bench.Commands() {
		if strings.HasPrefix(c.Name, prefix) {
			cs = append(cs, editor.Candidate{Insert: ":" + c.Name, Hint: c.Usage + "  " + c.About})
		}
	}
	return cs
}

// topics are what :help answers to, from the catalogue: names (`trim`, `|`, `@csv`) and
// sections (`math`), by prefix, any case.
func topics(prefix string) []editor.Candidate {
	c, err := catalog.Load()
	if err != nil {
		return nil
	}
	ns := append(c.Names(), "jq", "functions")
	for _, s := range c.Sections() {
		ns = append(ns, strings.ToLower(s))
	}
	var cs []editor.Candidate
	for _, n := range dedupe(ns) {
		if strings.HasPrefix(strings.ToLower(n), strings.ToLower(prefix)) {
			cs = append(cs, editor.Candidate{Insert: n, Hint: hint(n)})
		}
	}
	return cs
}

// jqWord completes the name or $variable the cursor is in or just after, from expr's tokens.
func jqWord(expr string, cur int) (from, to int, cands []editor.Candidate) {
	ts := jqsyntax.Lex(expr)
	rs := []rune(expr)
	for _, t := range ts {
		if t.Pos >= cur || cur > t.End {
			continue
		}
		word := string(rs[t.Pos:cur])
		switch {
		case (t.Kind == jqsyntax.Name || t.Kind == jqsyntax.Keyword) && !strings.Contains(t.Text, "::"):
			return t.Pos, t.End, names(word, ts)
		case t.Kind == jqsyntax.Variable:
			return t.Pos, t.End, variables(word, ts, t)
		}
		return cur, cur, nil
	}
	return cur, cur, nil
}

// names are jq's functions and keywords, and the defs written in the line, by prefix; a
// function that must have arguments comes with its (.
func names(prefix string, ts []jqsyntax.Token) []editor.Candidate {
	fs := functions()
	defs := lineDefs(ts)
	var ns []string
	for n := range fs {
		ns = append(ns, n)
	}
	for n := range defs {
		ns = append(ns, n)
	}
	ns = append(ns, keywords...)
	var cs []editor.Candidate
	for _, n := range sortByPrefix(dedupe(ns), prefix) {
		c := editor.Candidate{Insert: n, Hint: hint(n)}
		if d, ok := defs[n]; ok {
			c.Hint = d.sig + "  (defined in this line)"
			if d.args {
				c.Insert += "("
			}
		} else if a, ok := fs[n]; ok && a > 0 {
			c.Insert += "("
		}
		c.Show = n
		cs = append(cs, c)
	}
	return cs
}

// variables are $ENV, the REPL's $root and $_, and the ones the line binds, by prefix.
func variables(prefix string, ts []jqsyntax.Token, at jqsyntax.Token) []editor.Candidate {
	known := map[string]string{
		"$ENV":  "the environment, an object of its variables",
		"$root": "the whole document",
		"$_":    "the last line's result (an array if it gave several)",
	}
	for _, t := range ts {
		if t.Kind == jqsyntax.Variable && t.Pos != at.Pos {
			if _, ok := known[t.Text]; !ok {
				known[t.Text] = "bound in this line"
			}
		}
	}
	var ns []string
	for n := range known {
		ns = append(ns, n)
	}
	var cs []editor.Candidate
	for _, n := range sortByPrefix(ns, prefix) {
		cs = append(cs, editor.Candidate{Insert: n, Hint: known[n]})
	}
	return cs
}

// keywords that start something worth completing: not then/elif/else/end/catch, which follow.
var keywords = []string{"def", "if", "reduce", "foreach", "try", "label", "and", "or",
	"as", "import", "include", "null", "true", "false"}

type def struct {
	sig  string
	args bool
}

// lineDefs are the defs written in the line: def name or def name(params).
func lineDefs(ts []jqsyntax.Token) map[string]def {
	ds := map[string]def{}
	var code []jqsyntax.Token
	for _, t := range ts {
		if t.Kind != jqsyntax.Space && t.Kind != jqsyntax.Comment {
			code = append(code, t)
		}
	}
	for i := 0; i+1 < len(code); i++ {
		if code[i].Kind != jqsyntax.Keyword || code[i].Text != "def" || code[i+1].Kind != jqsyntax.Name {
			continue
		}
		d := def{sig: "def " + code[i+1].Text}
		if i+2 < len(code) && code[i+2].Text == "(" {
			d.args = true
			var b strings.Builder
			for _, t := range code[i+2:] {
				b.WriteString(t.Text)
				if t.Text == ")" && t.Depth == code[i+2].Depth {
					break
				}
			}
			d.sig += b.String()
		}
		ds[code[i+1].Text] = d
	}
	return ds
}

var (
	funcsOnce sync.Once
	funcs     map[string]int // gojq's functions, by the fewest arguments each takes
)

// functions are gojq's builtins, not the internal ones (_name), by the fewest arguments they
// take: 0 if it can be called bare.
func functions() map[string]int {
	funcsOnce.Do(func() {
		funcs = map[string]int{}
		q, _ := gojq.Parse(`builtins[]`)
		code, err := gojq.Compile(q)
		if err != nil {
			return
		}
		it := code.Run(nil)
		for v, ok := it.Next(); ok; v, ok = it.Next() {
			s, _ := v.(string)
			name, arity, _ := strings.Cut(s, "/")
			if name == "" || strings.HasPrefix(name, "_") {
				continue
			}
			n := 0
			for _, c := range arity {
				n = n*10 + int(c-'0')
			}
			if a, ok := funcs[name]; !ok || n < a {
				funcs[name] = n
			}
		}
	})
	return funcs
}

// hint is the manual's title for a name and the first sentence of its text.
func hint(name string) string {
	c, err := catalog.Load()
	if err != nil {
		return ""
	}
	es, exact := c.Lookup(name)
	if !exact || len(es) == 0 {
		if a, ok := functions()[name]; ok {
			if a == 0 {
				return name + ": gojq has it; the manual has no entry"
			}
			return name + "(…): gojq has it; the manual has no entry"
		}
		return ""
	}
	h := catalog.Plain(es[0].Title)
	body := strings.ReplaceAll(strings.TrimSpace(es[0].Body), "`", "")
	body = strings.Join(strings.Fields(body), " ")
	if i := strings.Index(body, ". "); i >= 0 {
		body = body[:i+1]
	}
	if r := []rune(body); len(r) > 100 {
		body = string(r[:99]) + "…"
	}
	if body != "" {
		h += ": " + body
	}
	return h
}

// sortByPrefix keeps the names that start with prefix, the exact one first, then in order.
func sortByPrefix(ns []string, prefix string) []string {
	var out []string
	for _, n := range ns {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i] == prefix) != (out[j] == prefix) {
			return out[i] == prefix
		}
		return out[i] < out[j]
	})
	return out
}

func dedupe(ns []string) []string {
	slices.Sort(ns)
	return slices.Compact(ns)
}
