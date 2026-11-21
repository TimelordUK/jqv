package assist

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/scope"
	"github.com/TimelordUK/jqv/internal/shape"
)

// ContextHint is the context line (Q-047, DESIGN §22): under the line as you type, what `.`
// is at the cursor, as the program that gives it and its shape one level deep. On a key
// that is there (`.projects▮`) it is what the key holds; while a key is being started
// (`.projects[0].▮`) or one not there typed, the object it is picked from; with no term
// (after `|`, in `select(`), `.` itself. Nothing on a command, in a comment, or where `.`
// depends on a caller.
func ContextHint(s *bench.Session) func(line string, cur int) string {
	return func(line string, cur int) string {
		off, expr, ok := jqPart(line)
		if !ok || cur < off || strings.TrimSpace(expr) == "" {
			return ""
		}
		c := cur - off
		for _, t := range jqsyntax.Lex(expr) {
			if t.Kind == jqsyntax.Comment && t.Pos < c && c <= t.End {
				return ""
			}
		}
		sc := scope.At(expr, c)
		shapes := s.Shapes()
		program, known := sc.Value()
		if sc.Term.Typing && sc.Term.Key != "" {
			if here, ok := sc.Here(); ok {
				if f := shapes.Of(here); f.Shape != nil && f.Missing == "" {
					program = here
				}
			}
		}
		if !known {
			return ""
		}
		f := shapes.Of(program)
		if f.Err != nil && strings.HasPrefix(f.Err.Error(), "variable not defined") {
			// the program ran without the `as` that binds it (past a def, say): the engine's
			// limit, not the line's mistake, so nothing is said rather than a wrong error
			return ""
		}
		if r := []rune(program); len(r) > 40 {
			program = "…" + string(r[len(r)-39:]) // shown shorter; found on the whole of it
		}
		return program + "  " + found(f, max(40, 96-len([]rune(program))))
	}
}

// found is what a program gives in brief, in about width runes, for the context line: its
// shape, how it failed, whether the run was cut short.
func found(f scope.Found, width int) string {
	switch {
	case f.Missing != "":
		return "no " + f.Missing + " here"
	case f.Shape != nil:
		out := shape.Brief(f.Shape, width)
		if f.Cut {
			out += "  (of the first outputs)"
		}
		return out
	case f.Err != nil:
		e := f.Err.Error()
		if r := []rune(e); len(r) > 60 {
			e = string(r[:59]) + "…"
		}
		return "error: " + e
	}
	return "nothing"
}

// Hints are hinters tried in turn, the first with something to say shown.
func Hints(hs ...func(line string, cur int) string) func(line string, cur int) string {
	return func(line string, cur int) string {
		for _, h := range hs {
			if s := h(line, cur); s != "" {
				return s
			}
		}
		return ""
	}
}

// Stack is F3: the stack of frames at the cursor, :scope with the cursor marked, printed
// above; the line stays as it is.
func Stack(s *bench.Session) func(line string, cur int) (string, int) {
	return func(line string, cur int) (string, int) {
		off, expr, ok := jqPart(line)
		if !ok || strings.TrimSpace(expr) == "" {
			return "F3 shows what . is at the cursor, on a line of jq\n", cur
		}
		rs := []rune(expr)
		c := min(max(cur-off, 0), len(rs))
		return s.Do(":scope " + string(rs[:c]) + "▮" + string(rs[c:])).Text, cur
	}
}

// RecursionHint is the context line on recursion (Q-052): on a recursive def's name, how
// often it calls itself; on one of those calls, which it is; on recurse, walk or .., what
// the builtin does. "" anywhere else.
func RecursionHint(line string, cur int) string {
	off, expr, ok := jqPart(line)
	if !ok || cur < off {
		return ""
	}
	c := cur - off
	on := func(sp [2]int) bool { return sp[0] <= c && c <= sp[1] }
	r := jqsyntax.Recursive(expr)
	for _, d := range r.Defs {
		n := len(d.Calls)
		if on(d.Name) {
			return fmt.Sprintf("↻ %s is recursive: it calls itself in %s, each a level deeper", d.Def.Text, plural(n, "place"))
		}
		for i, call := range d.Calls {
			if on(call) {
				return fmt.Sprintf("↻ %s calls itself here (%d of %d): recursion", d.Def.Text, i+1, n)
			}
		}
	}
	rs := []rune(expr)
	for _, b := range r.Builtins {
		if on(b) {
			switch string(rs[b[0]:b[1]]) {
			case "walk":
				return "↻ walk(f): recursive, f applied to every value from the leaves up"
			case "..":
				return "↻ ..: recurse, . and every value under it, depth first"
			}
			return "↻ recurse: recursive, . and every value under it, depth first (recurse(f) follows f)"
		}
	}
	return ""
}
