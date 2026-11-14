package bench

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TimelordUK/jqv/internal/query"
	"github.com/TimelordUK/jqv/internal/scope"
	"github.com/TimelordUK/jqv/internal/shape"
)

// What scope may spend finding a program's shape by running it (DESIGN §22): it runs as
// you type, so far less than a line's 5 s and 50 outputs to show, but enough outputs to
// see a place's keys and values. A run cut short says so.
const (
	ScopeTimeout = 150 * time.Millisecond
	ScopeMax     = 1000
)

// Shapes finds what programs give on `.`, the document after the stages, for scope: the
// document's own shape walked when there are no stages, else runs, bounded and kept until
// the stages or $_ change.
func (s *Session) Shapes() *scope.Shapes {
	on := s.Program("")
	if s.shapes != nil && s.shapesOn == on {
		return s.shapes
	}
	if s.keyOrder == nil && s.Doc != nil {
		s.keyOrder = shape.KeyOrder(s.Doc)
	}
	sh := &scope.Shapes{Order: s.keyOrder, Run: func(p string) ([]any, bool, error) {
		vars := map[string]any{"$root": s.root, "$_": s.lastVal}
		r := query.Run(s.Program(p), s.root, vars, query.Options{Timeout: ScopeTimeout, Max: ScopeMax})
		return r.Values, r.More || r.Late, r.Err
	}}
	if len(s.stages) == 0 {
		sh.Input = s.documentShape()
	}
	s.shapes, s.shapesOn = sh, on
	return sh
}

// documentShape is the document tree's shape, made once; nil when there is no tree.
func (s *Session) documentShape() *shape.Shape {
	if s.docShape == nil && s.Doc != nil {
		s.docShape = shape.FromNode(s.Doc)
	}
	return s.docShape
}

// scope is the :scope command (Q-045, DESIGN §22): what `.` is at the cursor of the last
// line or expr, as the stack of frames scope.At finds, each with the jq program that gives
// `.` there and, in brief, what that gives on the document; then the term being typed and
// the variables in reach. A ▮ in expr marks the cursor; without one it is at the end. For
// checking scope on real lines, as :lex is for the lexer, and turning a misread one into a
// test.
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
	shapes := s.Shapes()
	type row struct{ label, program, what string }
	var rows []row
	brief := func(program string, known bool) string {
		if !known {
			return "depends on the caller"
		}
		return found(shapes.Of(program))
	}
	for _, f := range sc.Frames {
		p := f.Input
		if p == "" {
			p = "?"
		}
		rows = append(rows, row{"  " + f.Label, p, brief(f.Input, f.Input != "")})
	}
	t := sc.Term
	typed := string([]rune(expr)[t.Pos:cur])
	p, ok := sc.Value()
	what := brief(p, ok)
	if !ok {
		p = "?"
	}
	switch {
	case !ok:
	case sc.Key:
		what = "an object's key, on " + what
	case t.Typing:
		what = "a key of " + what
	}
	rows = append(rows, row{"▮ " + typed, p, what})
	var vs []string
	for v := range sc.Vars {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	for _, v := range vs {
		rows = append(rows, row{"  " + v, sc.Vars[v], brief(sc.Vars[v], true)})
	}
	w1, w2 := 0, 0
	for i := range rows {
		rows[i].label = clip(rows[i].label, 32)
		rows[i].program = clip(rows[i].program, 40)
		w1 = max(w1, len([]rune(rows[i].label)))
		w2 = max(w2, len([]rune(rows[i].program)))
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %s", w1, r.label, w2, r.program, r.what), " ") + "\n")
	}
	return b.String()
}

// found is what a program gives, in brief: its shape one level deep, what is missing, how
// it failed, and whether the run was cut short.
func found(f scope.Found) string {
	var parts []string
	switch {
	case f.Missing != "":
		parts = append(parts, "no "+f.Missing+" here")
	case f.Shape != nil:
		parts = append(parts, shape.Brief(f.Shape, 70))
	case f.Err == nil:
		parts = append(parts, "nothing")
	}
	if f.Absent > 0 {
		parts = append(parts, fmt.Sprintf("(missing in %d: null there)", f.Absent))
	}
	if f.Err != nil {
		parts = append(parts, "error: "+clip(f.Err.Error(), 60))
	}
	if f.Cut {
		parts = append(parts, fmt.Sprintf("(of the first outputs: stopped at %d or %s)", ScopeMax, ScopeTimeout))
	}
	return strings.Join(parts, "  ")
}

// clip shortens t to n runes, ending in … when cut.
func clip(t string, n int) string {
	if r := []rune(t); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return t
}
