package bench

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/query"
	"github.com/itchyny/gojq"
)

// The :tree command (Q-005, Q-004): a line's parse tree, with what flows out of each step on
// the document. `.[].people[]` is one term with three steps; `a | b` is two parts, each run
// with the parts before it, so the tree shows where values multiply, vanish or fail.

// row is one line of the tree: what was written, what it does, and the program that runs
// up to and including it (after the stages).
type row struct {
	depth       int
	last        []bool // for each depth, whether this row's branch is the last at that depth
	label, desc string
	upto        string
	group       bool // a part whose steps follow as its children
}

func (s *Session) tree(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to show: run a line first, or :tree expr\n"
	}
	q, err := gojq.Parse(expr)
	if err != nil {
		return failure(expr, err)
	}
	var rows []row
	parts := pipeParts(q)
	for i, p := range parts {
		before := make([]string, i)
		for j := range before {
			before[j] = parts[j].String()
		}
		upto := func(text string) string {
			return strings.Join(append(before, text), " | ")
		}
		lastPart := i == len(parts)-1
		steps := termSteps(p)
		if len(parts) == 1 && len(steps) > 1 { // one term: its steps are the tree
			for k, st := range steps {
				rows = append(rows, row{depth: 1, last: []bool{k == len(steps)-1}, label: st.label, desc: st.desc, upto: upto(st.text)})
			}
			continue
		}
		if len(steps) > 1 {
			rows = append(rows, row{depth: 1, last: []bool{lastPart}, label: p.String(), upto: upto(p.String()), group: true})
			for k, st := range steps {
				rows = append(rows, row{depth: 2, last: []bool{lastPart, k == len(steps)-1}, label: st.label, desc: st.desc, upto: upto(st.text)})
			}
			continue
		}
		desc := describeQuery(p)
		if len(steps) == 1 {
			desc = steps[0].desc
		}
		rows = append(rows, row{depth: 1, last: []bool{lastPart}, label: p.String(), desc: desc, upto: upto(p.String())})
	}

	// Lay out: the branches and label in one column, the description in the next, the result after.
	lefts := make([]string, len(rows))
	lw, dw := 0, 0
	for i, r := range rows {
		var b strings.Builder
		for d := 0; d < r.depth-1; d++ {
			if r.last[d] {
				b.WriteString("   ")
			} else {
				b.WriteString("│  ")
			}
		}
		if r.last[r.depth-1] {
			b.WriteString("└─ ")
		} else {
			b.WriteString("├─ ")
		}
		b.WriteString(r.label)
		lefts[i] = b.String()
		lw = max(lw, len([]rune(lefts[i])))
		dw = max(dw, len([]rune(r.desc)))
	}
	var b strings.Builder
	b.WriteString(q.String() + "\n")
	failed := false // after the first step that fails, the rest are not reached
	for i, r := range rows {
		res := s.run(r.upto)
		got := summary(res)
		switch {
		case failed:
			got = "not reached"
		case r.group && res.Err != nil:
			got = "fails, below"
		case res.Err != nil:
			failed = true
			h := funcAsField(r.label)
			if h == "" {
				h = query.Hint(res.Err)
			}
			if h != "" {
				got += "\n" + strings.Repeat(" ", lw+dw+6) + "hint: " + h
			}
		}
		line := pad(lefts[i], lw) + "  " + pad(r.desc, dw) + "  → " + got
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return b.String()
}

func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-len([]rune(s))))
}

// pipeParts flattens a | b | c into its parts. A binding (`. as $x | …`) or a query with
// definitions is kept whole: its left side means nothing on its own.
func pipeParts(q *gojq.Query) []*gojq.Query {
	if q.Op == gojq.OpPipe && len(q.Patterns) == 0 && len(q.FuncDefs) == 0 && q.Meta == nil && len(q.Imports) == 0 {
		return append(pipeParts(q.Left), pipeParts(q.Right)...)
	}
	return []*gojq.Query{q}
}

type step struct{ label, desc, text string }

// termSteps splits a plain term into its steps: `.[0].people[]` is `.[0]`, `.people`, `[]`.
// Each step's text is the term up to and including it, so it can be run.
func termSteps(q *gojq.Query) []step {
	t := q.Term
	if t == nil || q.Op != 0 || len(q.FuncDefs) > 0 {
		return nil
	}
	var steps []step
	base := *t
	base.SuffixList = nil
	if t.Type != gojq.TermTypeIdentity || len(t.SuffixList) == 0 {
		steps = append(steps, step{base.String(), describeTerm(&base), base.String()})
	}
	for k, sf := range t.SuffixList {
		upto := *t
		upto.SuffixList = t.SuffixList[:k+1]
		label := sf.String()
		if k == 0 && t.Type == gojq.TermTypeIdentity {
			label = upto.String() // `.[]`, not `[]`, when it starts the term
		}
		steps = append(steps, step{label, describeSuffix(sf), upto.String()})
	}
	return steps
}

func describeSuffix(sf *gojq.Suffix) string {
	d := "each element"
	if sf.Index != nil {
		d = describeIndex(sf.Index)
	}
	if sf.Optional {
		d += ", errors skipped"
	}
	return d
}

func describeIndex(ix *gojq.Index) string {
	switch {
	case ix.Name != "":
		return fmt.Sprintf("field %q", ix.Name)
	case ix.Str != nil:
		return "field " + ix.Str.String()
	case ix.IsSlice:
		return "slice: an array of some elements"
	case ix.Start != nil:
		return "element " + ix.Start.String()
	}
	return "index"
}

func describeTerm(t *gojq.Term) string {
	switch t.Type {
	case gojq.TermTypeIdentity:
		return "the input"
	case gojq.TermTypeRecurse:
		return "every value inside, recursively"
	case gojq.TermTypeIndex:
		return describeIndex(t.Index)
	case gojq.TermTypeFunc:
		if d, ok := funcs[t.Func.Name]; ok {
			return d
		}
		return fmt.Sprintf("function %s/%d", t.Func.Name, len(t.Func.Args))
	case gojq.TermTypeObject:
		return "build an object"
	case gojq.TermTypeArray:
		return "collect into an array"
	case gojq.TermTypeQuery:
		return "a group in ( )"
	case gojq.TermTypeIf:
		return "if / then / else"
	case gojq.TermTypeTry:
		return "try: errors caught"
	case gojq.TermTypeReduce:
		return "reduce: fold into one value"
	case gojq.TermTypeForeach:
		return "foreach: fold, giving each step"
	case gojq.TermTypeNull, gojq.TermTypeTrue, gojq.TermTypeFalse,
		gojq.TermTypeNumber, gojq.TermTypeString:
		return "a value"
	}
	return ""
}

func describeQuery(q *gojq.Query) string {
	if q.Term != nil && q.Op == 0 {
		return describeTerm(q.Term)
	}
	if len(q.Patterns) > 0 {
		return "binds a variable"
	}
	switch q.Op {
	case gojq.OpComma:
		return "both: the left's outputs, then the right's"
	case gojq.OpAlt:
		return "the left, or the right if that gives only null/false"
	case gojq.OpAnd, gojq.OpOr:
		return "a test"
	case gojq.OpEq, gojq.OpNe, gojq.OpGt, gojq.OpLt, gojq.OpGe, gojq.OpLe:
		return "a comparison"
	case gojq.OpAdd, gojq.OpSub, gojq.OpMul, gojq.OpDiv, gojq.OpMod:
		return "arithmetic"
	case gojq.OpAssign, gojq.OpModify, gojq.OpUpdateAdd, gojq.OpUpdateSub, gojq.OpUpdateMul,
		gojq.OpUpdateDiv, gojq.OpUpdateMod, gojq.OpUpdateAlt:
		return "an update: the input with a part changed"
	}
	return ""
}

// funcs says in a few words what common functions do, until the catalogue (Q-008) does.
var funcs = map[string]string{
	"select":            "keep . where the test is true",
	"map":               "apply to each element, collect",
	"map_values":        "apply to each value, keep the shape",
	"length":            "how many (or string length)",
	"keys":              "an object's keys, sorted",
	"has":               "does it have this key",
	"to_entries":        "object → [{key, value}]",
	"from_entries":      "[{key, value}] → object",
	"with_entries":      "change each key/value pair",
	"add":               "sum, concatenate or merge",
	"first":             "the first output",
	"last":              "the last output",
	"limit":             "at most n outputs",
	"range":             "count: a number per output",
	"empty":             "no output at all",
	"sort":              "sort an array",
	"sort_by":           "sort an array by a key",
	"group_by":          "arrays of elements with the same key",
	"unique":            "sorted, without duplicates",
	"unique_by":         "one element per key",
	"min":               "the smallest",
	"max":               "the largest",
	"reverse":           "reverse an array or string",
	"flatten":           "nested arrays into one",
	"any":               "is any true",
	"all":               "are all true",
	"not":               "true ↔ false",
	"type":              "the type's name",
	"tostring":          "to a string",
	"tonumber":          "to a number",
	"tojson":            "to JSON text",
	"fromjson":          "from JSON text",
	"test":              "does a regex match",
	"match":             "regex match details",
	"capture":           "regex named groups → object",
	"contains":          "is it contained",
	"inside":            "is it inside",
	"startswith":        "starts with this text",
	"endswith":          "ends with this text",
	"ascii_downcase":    "to lower case",
	"ascii_upcase":      "to upper case",
	"split":             "string → array of parts",
	"join":              "array of strings → one string",
	"trim":              "strip whitespace at both ends",
	"ltrimstr":          "remove this prefix",
	"rtrimstr":          "remove this suffix",
	"paths":             "the path to every value",
	"leaf_paths":        "the path to every scalar",
	"getpath":           "the value at a path",
	"del":               "the input without this part",
	"recurse":           "every value inside, recursively",
	"env":               "environment variables",
	"input":             "the next input",
	"debug":             "print to stderr, pass through",
	"error":             "fail with this message",
	"IN":                "is it one of these",
	"index":             "first position of",
	"indices":           "every position of",
	"splits":            "regex split, as outputs",
	"sub":               "regex replace, first",
	"gsub":              "regex replace, all",
	"ascii":             "a code point to a character",
	"explode":           "string → code points",
	"implode":           "code points → string",
	"floor":             "round down",
	"ceil":              "round up",
	"round":             "round to nearest",
	"fabs":              "absolute value",
	"abs":               "absolute value",
	"sqrt":              "square root",
	"tostream":          "into [path, leaf] events",
	"fromstream":        "events back into values",
	"input_line_number": "the input's line number",
	"walk":              "apply bottom-up to every value",
	"transpose":         "rows ↔ columns",
	"objects":           "only objects",
	"arrays":            "only arrays",
	"strings":           "only strings",
	"numbers":           "only numbers",
	"scalars":           "only non-containers",
	"values":            "only non-null",
	"nulls":             "only null",
	"isempty":           "does it give no output",
	"until":             "repeat until true",
	"while":             "repeat while true",
	"repeat":            "apply again and again",
}
