package bench

import (
	"fmt"
	"sort"
	"strings"

	"github.com/TimelordUK/jqv/internal/scope"
)

// scope is the :scope command (Q-045, DESIGN §22): what `.` is at the cursor of the last
// line or expr, as the stack of frames scope.At finds, each with the jq program that gives
// `.` there; then the term being typed and the variables in reach. A ▮ in expr marks the
// cursor; without one it is at the end. For checking scope on real lines, as :lex is for
// the lexer, and turning a misread one into a test.
func (s *Session) scope(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to show: run a line first, or :scope expr (▮ marks the cursor)\n"
	}
	cur := len([]rune(expr))
	if i := strings.Index(expr, "▮"); i >= 0 {
		expr = expr[:i] + expr[i+len("▮"):]
		cur = len([]rune(expr[:i]))
	}
	sc := scope.At(expr, cur)
	type row struct{ label, input, note string }
	var rows []row
	for _, f := range sc.Frames {
		in := f.Input
		if in == "" {
			in = "?"
		}
		rows = append(rows, row{"  " + f.Label, in, ""})
	}
	t := sc.Term
	typed := string([]rune(expr)[t.Pos:cur])
	in, ok := sc.Value()
	if !ok {
		in = "?"
	}
	note := ""
	switch {
	case !sc.Known && (t.Head == "" || t.Head[0] != '$'):
		note = "depends on the caller"
	case sc.Key:
		note = "an object's key"
	case t.Typing && t.Path == "" && t.Head == "":
		note = "a key of ."
	case t.Typing:
		note = "a key of " + t.Head + t.Path
	}
	rows = append(rows, row{"▮ " + typed, in, note})
	var vs []string
	for v := range sc.Vars {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	for _, v := range vs {
		rows = append(rows, row{"  " + v, sc.Vars[v], "bound"})
	}
	w := 0
	for i, r := range rows {
		if l := []rune(r.label); len(l) > 40 {
			rows[i].label = string(l[:39]) + "…"
		}
		w = max(w, len([]rune(rows[i].label)))
	}
	var b strings.Builder
	for _, r := range rows {
		line := fmt.Sprintf("%-*s  %s", w, r.label, r.input)
		if r.note != "" {
			line += "   " + r.note
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return b.String()
}
