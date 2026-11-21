package scope

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// shorthands are the object entries that write a key twice, `"name": .name` or `e: $e`, which
// jq lets you write once, `{name}` and `{$e}`, as C# and JS do (owner 2026-11-21). A note on
// the entry: said on the context line when the cursor is on it, never drawn.
func shorthands(line string) []Diag {
	var ds []Diag
	jqsyntax.Walk(jqsyntax.Parse(line), func(n *jqsyntax.Node) bool {
		if n.Kind != jqsyntax.NEntry || n.Open || len(n.Kids) != 2 {
			return true
		}
		key, val := keyName(n.Kids[0]), n.Kids[1]
		if key == "" || len(val.Kids) > 0 {
			return true
		}
		var short string
		switch {
		case val.Kind == jqsyntax.NField && val.Text == "."+key:
			short = key
		case val.Kind == jqsyntax.NVar && val.Text == "$"+key:
			short = "$" + key
		default:
			return true
		}
		if ts := jqsyntax.Lex(key); len(ts) != 1 || ts[0].Kind == jqsyntax.Keyword { // gojq takes {if}, older jq doesn't
			return true
		}
		ds = append(ds, Diag{From: n.Pos, To: n.End, Level: Note, Suggest: []string{short},
			Msg: fmt.Sprintf("can be just %s: {%s} is {%s: %s}", short, short, key, val.Text)})
		return true
	})
	return ds
}

// keyName is an entry's key when it is a plain name, written bare or quoted; "" otherwise.
func keyName(k *jqsyntax.Node) string {
	switch {
	case k.Kind == jqsyntax.NKey && !strings.HasPrefix(k.Text, "$"):
		return k.Text
	case k.Kind == jqsyntax.NString && len(k.Kids) == 1 && k.Kids[0].Kind == jqsyntax.NStrPart:
		s := k.Kids[0].Text
		for i, r := range s {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				return ""
			}
		}
		return s
	}
	return ""
}
