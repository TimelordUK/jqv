package pick

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/itchyny/gojq"
)

// Kind is what a stage is: which gesture wrote it and can change it (DESIGN §28.1).
type Kind uint8

const (
	Opaque Kind = iota // text no gesture reads, kept word for word
	Picks              // the grain and the columns, one gesture's: `.[] | {Job, Status}`
	Filter             // select(cond)
)

// Stage is one step of a pipeline. Text is what it is; the rest is what its kind's reader
// found in it.
type Stage struct {
	Kind Kind
	Text string
	Form Form   // Picks
	Cond string // Filter: what is inside select(...)
}

// Pipeline is a program as stages joined by top-level pipes (DESIGN §28, R-012). It is read
// from the text each time and written back to it; nothing is kept beside the text.
type Pipeline []Stage

// Parse reads a program as stages. Picks may be only the first stage, as they lift from the
// document. A program it can't split safely is one opaque stage, so no text is ever lost or
// changed; "" is no stages.
func Parse(program string) Pipeline {
	program = strings.TrimSpace(program)
	if program == "" {
		return nil
	}
	parts, ok := split(program)
	if !ok {
		return Pipeline{{Kind: Opaque, Text: program}}
	}
	var p Pipeline
	for i := 0; i < len(parts); i++ {
		if i == 0 && len(parts) > 1 {
			text := parts[0] + " | " + parts[1]
			if f, err := Read(text); err == nil {
				p = append(p, Stage{Kind: Picks, Text: text, Form: f})
				i++
				continue
			}
		}
		p = append(p, stage(parts[i]))
	}
	return p
}

// split cuts a program at its top-level pipes. Every part must parse on its own, or be a
// binding the rest of the pipe is the body of (`.prizes[] as $p`); anything else (a pipe
// inside if…end or a def's body) makes it refuse, and the program stays whole.
func split(program string) ([]string, bool) {
	if _, err := gojq.Parse(program); err != nil {
		return nil, false
	}
	rs := []rune(program)
	var parts []string
	from := 0
	for _, t := range jqsyntax.Lex(program) {
		if t.Kind == jqsyntax.Pipe && t.Depth == 0 {
			parts = append(parts, strings.TrimSpace(string(rs[from:t.Pos])))
			from = t.End
		}
	}
	parts = append(parts, strings.TrimSpace(string(rs[from:])))
	for i, s := range parts {
		if _, err := gojq.Parse(s); err == nil {
			continue
		}
		if _, err := gojq.Parse(s + " | ."); err == nil && i < len(parts)-1 {
			continue
		}
		return nil, false
	}
	return parts, true
}

// stage recognises one part.
func stage(s string) Stage {
	if c, ok := selectCond(s); ok {
		return Stage{Kind: Filter, Text: s, Cond: c}
	}
	return Stage{Kind: Opaque, Text: s}
}

// selectCond is the condition of a part that is exactly `select(cond)`.
func selectCond(s string) (string, bool) {
	q, err := gojq.Parse(s)
	if err != nil || q.Op != 0 || len(q.FuncDefs) > 0 || q.Term == nil || q.Term.Type != gojq.TermTypeFunc ||
		q.Term.Func.Name != "select" || len(q.Term.Func.Args) != 1 || len(q.Term.SuffixList) > 0 {
		return "", false
	}
	if !strings.HasPrefix(s, "select(") || !strings.HasSuffix(s, ")") {
		return "", false
	}
	return strings.TrimSpace(s[len("select(") : len(s)-1]), true
}

// String is the program.
func (p Pipeline) String() string { return p.Upto(len(p)) }

// Upto is the program of the first n stages: what goes into stage n. "" for n == 0, where the
// input is the document.
func (p Pipeline) Upto(n int) string {
	texts := make([]string, 0, n)
	for _, s := range p[:n] {
		texts = append(texts, s.Text)
	}
	return strings.Join(texts, " | ")
}

// Picks is the form of the pipeline's picks, its first stage. ok is false when the first stage
// is something else, so space can't change it; no stages is no picks, and ok.
func (p Pipeline) Picks() (f Form, ok bool) {
	if len(p) == 0 {
		return Form{}, true
	}
	return p[0].Form, p[0].Kind == Picks
}

// WithPicks is p with its picks replaced by f. Unpicking the last column takes the picks
// stage away, which is refused while later stages read its rows.
func (p Pipeline) WithPicks(f Form) (Pipeline, error) {
	if _, ok := p.Picks(); !ok {
		return p, errors.New("the program doesn't start with picks, so space can't change it")
	}
	switch {
	case f.Empty() && len(p) > 1:
		return p, errors.New("the stages after the picks need a column: u to undo them first")
	case f.Empty():
		return nil, nil
	}
	s := Stage{Kind: Picks, Text: Write(f), Form: f}
	if len(p) == 0 {
		return Pipeline{s}, nil
	}
	q := slices.Clone(p)
	q[0] = s
	return q, nil
}

// Where adds cond at the stage cursor at (DESIGN §28.3): to the filter there, joined by `and`
// or, with or set, `or` (DESIGN §29); on the empty slot after the last stage, as a new filter
// on the end. Conditions read left to right as they were added: and binds tighter than or in
// jq, so `A and B` then or C needs no brackets, and `A or B` then and C gets them.
//
// From daily use, two refusals: or on the end slot, where the tree shows only the rows that
// passed, so another value to or with can't be seen there; and a condition that is there
// already, as a filter of its own on the end or a term of the filter under the cursor.
func (p Pipeline) Where(at int, cond string, or bool) (Pipeline, error) {
	switch {
	case at == len(p) && or:
		return p, errors.New("| adds to a filter with or: K onto it, where the tree shows every row going in")
	case at == len(p) && slices.ContainsFunc(p, func(s Stage) bool { return s.Kind == Filter && same(s.Cond, cond) }):
		return p, fmt.Errorf("already filtered on %s: K onto the filter and | for another value", cond)
	case at >= 0 && at < len(p) && p[at].Kind == Filter && slices.ContainsFunc(terms(p[at].Cond), func(t string) bool { return same(t, cond) }):
		return p, fmt.Errorf("%s is in this filter already", cond)
	case at >= 0 && at < len(p) && p[at].Kind == Filter && opposite(cond) != "" && slices.ContainsFunc(terms(p[at].Cond), func(t string) bool { return same(t, opposite(cond)) }):
		return p, fmt.Errorf("%s is in this filter: it can't be both; u or D to start again", opposite(cond))
	case at == len(p):
		return append(slices.Clone(p), Stage{Kind: Filter, Text: "select(" + cond + ")", Cond: cond}), nil
	case at >= 0 && at < len(p) && p[at].Kind == Filter:
		old, op := p[at].Cond, " and "
		if or {
			op = " or "
		} else if q, err := gojq.Parse(old); err == nil && (q.Op == gojq.OpOr || q.Op == gojq.OpPipe) {
			old = "(" + old + ")" // `A | not and B` would be A | (not and B)
		}
		c := old + op + cond
		q := slices.Clone(p)
		q[at] = Stage{Kind: Filter, Text: "select(" + c + ")", Cond: c}
		return q, nil
	}
	return p, errors.New("a filter goes on the end for now: J to the end, or onto a filter to add to it")
}

// opposite is a condition with == and != swapped, for spotting `.a == 1` beside `.a != 1`;
// "" when it is neither.
func opposite(cond string) string {
	q, err := gojq.Parse(cond)
	if err != nil || (q.Op != gojq.OpEq && q.Op != gojq.OpNe) {
		return ""
	}
	op := gojq.OpNe
	if q.Op == gojq.OpNe {
		op = gojq.OpEq
	}
	return (&gojq.Query{Left: q.Left, Op: op, Right: q.Right}).String()
}

// Not turns the filter at round, or back (~, Q-073): `select(c)` becomes `select((c) | not)`,
// everything c let through left out, and `select((c) | not)` becomes `select(c)` again. The
// brackets go round and and or, which don't need them, so the not reads as on the whole.
func (p Pipeline) Not(at int) (Pipeline, error) {
	if at < 0 || at >= len(p) || p[at].Kind != Filter {
		return p, errors.New("~ turns a filter round: K onto a filter")
	}
	c := p[at].Cond
	if inner, ok := unNot(c); ok {
		c = inner
	} else if q, err := gojq.Parse(c); err == nil && (q.Op == gojq.OpAnd || q.Op == gojq.OpOr || q.Op == gojq.OpPipe || q.Op == gojq.OpComma || q.Op == gojq.OpAlt) {
		c = "(" + c + ") | not"
	} else {
		c += " | not"
	}
	q := slices.Clone(p)
	q[at] = Stage{Kind: Filter, Text: "select(" + c + ")", Cond: c}
	return q, nil
}

// unNot is c without a not on the end, and its brackets: `(A and B) | not` is `A and B`; ok
// is false when c doesn't end in one.
func unNot(c string) (string, bool) {
	q, err := gojq.Parse(c)
	if err != nil || q.Op != gojq.OpPipe || q.Right.String() != "not" {
		return "", false
	}
	t := strings.TrimSpace(c)
	t = strings.TrimSpace(strings.TrimSuffix(t, "not"))
	t = strings.TrimSpace(strings.TrimSuffix(t, "|"))
	if l, err := gojq.Parse(t); err == nil && l.Op == 0 && l.Term != nil && l.Term.Type == gojq.TermTypeQuery && len(l.Term.SuffixList) == 0 {
		t = strings.TrimSpace(t[1 : len(t)-1]) // (…): one bracketed query, nothing after
	}
	return t, true
}

// same says whether two conditions are the same jq, spacing aside.
func same(a, b string) bool {
	qa, errA := gojq.Parse(a)
	qb, errB := gojq.Parse(b)
	if errA != nil || errB != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	return qa.String() == qb.String()
}

// terms are the conditions and and or join in cond, brackets opened: `A or (B and C)` is A,
// B and C.
func terms(cond string) []string {
	q, err := gojq.Parse(cond)
	if err != nil {
		return []string{cond}
	}
	var out []string
	var walk func(q *gojq.Query)
	walk = func(q *gojq.Query) {
		switch {
		case q.Op == gojq.OpAnd || q.Op == gojq.OpOr:
			walk(q.Left)
			walk(q.Right)
		case q.Op == 0 && q.Term != nil && q.Term.Type == gojq.TermTypeQuery && len(q.Term.SuffixList) == 0:
			walk(q.Term.Query) // (…)
		default:
			out = append(out, q.String())
		}
	}
	walk(q)
	return out
}

// Delete takes away the stage at, the cursor's (D, R-014). The picks stay while later stages
// read their columns; the empty slot after the last stage has nothing to delete. Deleting a
// filter is always safe, as a filter keeps the rows' shape; checking what a later stage reads
// of anything else waits for scope.Check (DESIGN §28.5).
func (p Pipeline) Delete(at int) (Pipeline, error) {
	switch {
	case at < 0 || at >= len(p):
		return p, errors.New("D deletes the stage under the cursor: K onto one")
	case p[at].Kind == Picks && len(p) > 1:
		return p, errors.New("the stages after the picks read their columns: D those first")
	case len(p) == 1:
		return nil, nil
	}
	return slices.Delete(slices.Clone(p), at, at+1), nil
}

// Field is the keys from a record to n, where the records are the children of the tree's
// root: what a stage receives, one record per value. ok is false for a value inside an
// array of the record, or a whole container.
func Field(n *doc.Node) ([]string, error) {
	if n.IsContainer() {
		return nil, errors.New("= on a value, not a whole object or array")
	}
	if n.Parent == nil {
		return nil, errors.New("= on a value in one of the records")
	}
	var ks []string
	for c := n; c.Parent != nil && c.Parent.Parent != nil; c = c.Parent {
		if c.Parent.Kind != doc.Object {
			return nil, errors.New("= on a value inside an array of the record isn't written yet")
		}
		ks = append(ks, c.Key)
	}
	slices.Reverse(ks)
	return ks, nil
}

// Eq is the condition that a record's field at ks equals n's value: `.Status == "FAILURE"`.
// == compares any two values, so it needs none of GRID's type guards.
func Eq(ks []string, n *doc.Node) string { return path(ks) + " == " + literal(n) }

// Ne is Eq turned round (Q-071): the field is anything but n's value, `.Agent != "BLD-1"`.
// A record without the field passes, as null isn't the value.
func Ne(ks []string, n *doc.Node) string { return path(ks) + " != " + literal(n) }

// literal is n's value as jq: a string quoted, anything else as written.
func literal(n *doc.Node) string {
	if n.Kind == doc.String {
		b, _ := json.Marshal(n.Raw)
		return string(b)
	}
	return n.Raw
}
