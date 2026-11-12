package bench

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/TimelordUK/jqv/internal/catalog"
)

var (
	notDefined = regexp.MustCompile(`function not defined: ([A-Za-z_][A-Za-z0-9_]*)/(\d+)`)
	aritiesOf  = sync.OnceValue(catalog.Arities)
)

// arityHint is for a call of a function that exists, with the wrong number of arguments
// (owner 2026-11-12: `remainder(10)` said only `function not defined: remainder/1`). It says
// what the function takes, as the manual writes it; for a function that works on `.`, to pipe
// into it (`16 | sqrt`); and for the two-input math functions, which ignore `.`, how to pass
// the value coming in.
func arityHint(expr string, err error) string {
	m := notDefined.FindStringSubmatch(err.Error())
	if m == nil {
		return ""
	}
	name, called := m[1], m[2]
	as := aritiesOf()[name]
	if len(as) == 0 {
		return ""
	}
	c, _ := catalog.Load()
	var forms []string
	for _, n := range as {
		f := ""
		if c != nil {
			f = c.Signature(name, n)
		}
		if f == "" {
			f = name
			if n > 0 {
				f += "(" + strings.TrimSuffix(strings.Repeat("_; ", n), "; ") + ")"
			}
		}
		forms = append(forms, f)
	}
	arg := callArg(expr, name)
	h := fmt.Sprintf("%s takes %s, not %s", name, counts(as), called)
	switch {
	case len(as) == 1 && as[0] == 0:
		h += fmt.Sprintf(": it works on what comes in, so pipe the value into it: %s | %s", or(arg, "x"), name)
	case isMath2(c, name):
		h += fmt.Sprintf(": %s. It ignores what comes in; to use it, pass . as an argument: %s(.; %s)",
			strings.Join(forms, " or "), name, or(arg, "y"))
	default:
		h += ": " + strings.Join(forms, " or ")
	}
	return h + ". :help " + name
}

func or(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

// callArg is the one argument in the first call of name in expr (`16` in `sqrt(16)`), or ""
// when there is no such call or it has several arguments.
func callArg(expr, name string) string {
	loc := regexp.MustCompile(`(^|[^A-Za-z0-9_$])` + regexp.QuoteMeta(name) + `\s*\(`).FindStringIndex(expr)
	if loc == nil {
		return ""
	}
	depth := 0
	for i, r := range expr[loc[1]:] {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return strings.TrimSpace(expr[loc[1] : loc[1]+i])
			}
			depth--
		case ';':
			if depth == 0 {
				return ""
			}
		}
	}
	return ""
}

// counts is "2 arguments", "1, 2 or 3 arguments", "no arguments".
func counts(as []int) string {
	if len(as) == 1 && as[0] == 0 {
		return "no arguments"
	}
	ns := make([]string, len(as))
	for i, a := range as {
		ns[i] = strconv.Itoa(a)
	}
	s := ns[len(ns)-1]
	if len(ns) > 1 {
		s = strings.Join(ns[:len(ns)-1], ", ") + " or " + s
	}
	if len(as) == 1 && as[0] == 1 {
		return s + " argument"
	}
	return s + " arguments"
}

// isMath2 reports whether name is one of C's two-input math functions, which jq gives as
// two-argument functions that ignore `.` (the manual's Math section lists them).
func isMath2(c *catalog.Catalog, name string) bool {
	if c == nil {
		return false
	}
	for _, e := range c.Section("Math") {
		if _, list, ok := strings.Cut(e.Body, "Two-input C math functions:"); ok {
			list, _, _ = strings.Cut(list, "\n\n")
			return strings.Contains(list, "`"+name+"`")
		}
	}
	return false
}
