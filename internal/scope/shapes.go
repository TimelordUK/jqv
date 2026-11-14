package scope

import (
	"encoding/json"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/shape"
)

// Runner runs a jq program on the line's input, bounded by a timeout and a cap on outputs:
// what came out, and whether it was cut short by either.
type Runner func(program string) (vs []any, cut bool, err error)

// Shapes finds what the programs of a scope give, as shapes (DESIGN §22, step 2). A program
// that is keys and `[]` on arrays from `.` is walked on the input's shape, which counts
// exactly what jq would give and runs nothing. Anything else is run, and a path after it
// walked on what the run gave. Runs are kept by program text, so a line being typed runs
// each prefix once. One Shapes is good for as long as the line's input doesn't change.
type Shapes struct {
	Input *shape.Shape // the line input's shape; nil to run "." for it
	Run   Runner
	Order *shape.Order // the document's key order, for gojq's maps
	runs  map[string]Found
}

// Found is what a program gives.
type Found struct {
	Shape   *shape.Shape // nil when nothing came out
	Cut     bool         // a run stopped at its cap or timeout: the shape is of what came first
	Ran     bool         // jq was run, rather than the shape walked
	Err     error        // the run failed; Shape is of what came before
	Missing string       // the walk met a key no object at its place has: the step, `.x`
	Absent  int          // objects at the last key's place without it, which give null
}

// Of is what program gives on the line's input.
func (s *Shapes) Of(program string) Found {
	parts := pipeParts(program)
	k := len(parts)
	var steps [][]step
	for k > 0 {
		st, ok := pathSteps(parts[k-1])
		if !ok {
			break
		}
		steps = append([][]step{st}, steps...)
		k--
	}
	if k == len(parts) {
		return s.run(program)
	}
	var base Found
	if k == 0 {
		base = s.input()
	} else {
		base = s.run(strings.Join(parts[:k], " | "))
	}
	if base.Shape == nil {
		return base
	}
	var all []step
	for _, st := range steps {
		all = append(all, st...)
	}
	f, ok := walk(base.Shape, all)
	if !ok {
		return s.run(program)
	}
	f.Cut, f.Ran, f.Err = base.Cut, base.Ran, base.Err
	return f
}

func (s *Shapes) input() Found {
	if s.Input != nil {
		return Found{Shape: s.Input}
	}
	return s.run(".")
}

func (s *Shapes) run(program string) Found {
	if f, ok := s.runs[program]; ok {
		return f
	}
	if s.runs == nil {
		s.runs = map[string]Found{}
	}
	f := Found{Ran: true}
	if s.Run != nil {
		vs, cut, err := s.Run(program)
		f.Cut, f.Err = cut, err
		if len(vs) > 0 {
			f.Shape = shape.FromValues(vs, s.Order)
		}
	}
	s.runs[program] = f
	return f
}

// step is a key, or [] for an array's elements.
type step struct {
	key  string
	iter bool
}

// walk follows steps down a shape, as jq would down the values it describes. ok is false
// where the shape can't say exactly what jq would give (`[]` on objects, a key asked of an
// array or of nulls): then the program is run, which also gives jq's own error.
func walk(sh *shape.Shape, steps []step) (Found, bool) {
	absent := 0
	for i, st := range steps {
		objects, arrays := sh.Kinds[doc.Object], sh.Kinds[doc.Array]
		if st.iter {
			if arrays == 0 || arrays != sh.Count {
				return Found{}, false
			}
			sh = sh.Elements()
			if sh == nil { // only empty arrays
				return Found{}, true
			}
			absent = 0
			continue
		}
		if objects == 0 || objects != sh.Count {
			return Found{}, false
		}
		c := sh.Find(st.key)
		if c == nil {
			return Found{Missing: "." + shape.KeyText(st.key)}, true
		}
		if i < len(steps)-1 && c.Count < objects {
			return Found{}, false // the nulls would go on down the path: let jq say what they give
		}
		absent = objects - c.Count
		sh = c
	}
	return Found{Shape: sh, Absent: absent}, true
}

// pipeParts splits a program at its top-level pipes.
func pipeParts(program string) []string {
	rs := []rune(program)
	var parts []string
	from := 0
	for _, t := range jqsyntax.Lex(program) {
		if t.Kind == jqsyntax.Pipe && t.Depth == 0 {
			parts = append(parts, strings.TrimSpace(string(rs[from:t.Pos])))
			from = t.End
		}
	}
	return append(parts, strings.TrimSpace(string(rs[from:])))
}

// pathSteps reads a part that is only keys and []: `.`, `.a.b`, `.[]`, `."a b"[]?`. ok is
// false for anything else: an index, a slice, `..`, a variable, a call.
func pathSteps(part string) ([]step, bool) {
	var code []jqsyntax.Token
	for _, t := range jqsyntax.Lex(part) {
		if t.Kind != jqsyntax.Space {
			code = append(code, t)
		}
	}
	if len(code) == 0 || code[0].Kind != jqsyntax.Dot && code[0].Kind != jqsyntax.Field {
		return nil, false
	}
	var steps []step
	for i := 0; i < len(code); i++ {
		t := code[i]
		switch {
		case t.Kind == jqsyntax.Field && t.Text != ".":
			steps = append(steps, step{key: t.Text[1:]})
		case t.Kind == jqsyntax.Field: // ."a b"
			if i+1 >= len(code) || code[i+1].Kind != jqsyntax.Quote {
				return nil, false
			}
			j := i + 2
			var raw strings.Builder
			for j < len(code) && code[j].Kind != jqsyntax.Quote {
				if code[j].Kind != jqsyntax.Text && code[j].Kind != jqsyntax.Escape {
					return nil, false
				}
				raw.WriteString(code[j].Text)
				j++
			}
			if j == len(code) {
				return nil, false
			}
			var k string
			if json.Unmarshal([]byte(`"`+raw.String()+`"`), &k) != nil {
				return nil, false
			}
			steps = append(steps, step{key: k})
			i = j
		case t.Kind == jqsyntax.Dot:
			if i > 0 && (i+1 >= len(code) || code[i+1].Text != "[") { // the start, or .a.[]
				return nil, false
			}
		case t.Kind == jqsyntax.Bracket && t.Text == "[":
			if i+1 >= len(code) || code[i+1].Text != "]" {
				return nil, false
			}
			steps = append(steps, step{iter: true})
			i++
		case t.Kind == jqsyntax.Operator && t.Text == "?":
		default:
			return nil, false
		}
	}
	return steps, true
}
