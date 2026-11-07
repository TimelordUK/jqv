// Package query runs jq through gojq (Q-001): parse, compile with named variables, and run
// with a deadline and a cap on how many outputs are collected, so that a query typed in a
// REPL (`range(1e9)`, `repeat(1)`) can't hang or fill memory. Errors come back with a hint
// where a common mistake is behind them (Q-003).
package query

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"
)

// Options bound a run. Zero values mean no limit.
type Options struct {
	Timeout time.Duration
	Max     int // outputs to collect; Result.More says whether there were others
}

// Result is what a query gave.
type Result struct {
	Values []any
	More   bool  // stopped at Options.Max with more to come
	Err    error // a parse, compile or run error; Values holds what came before it
}

// Run parses and runs expr on input. vars are jq variables by name, with the $ ("$root").
func Run(expr string, input any, vars map[string]any, opt Options) Result {
	q, err := gojq.Parse(expr)
	if err != nil {
		return Result{Err: err}
	}
	names := make([]string, 0, len(vars))
	for n := range vars {
		names = append(names, n)
	}
	sort.Strings(names)
	values := make([]any, len(names))
	for i, n := range names {
		values[i] = vars[n]
	}
	code, err := gojq.Compile(q, gojq.WithVariables(names))
	if err != nil {
		return Result{Err: err}
	}
	ctx := context.Background()
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	var r Result
	it := code.RunWithContext(ctx, input, values...)
	for {
		v, ok := it.Next()
		if !ok {
			return r
		}
		if err, ok := v.(error); ok {
			if errors.Is(err, context.DeadlineExceeded) {
				err = errors.New("stopped after " + opt.Timeout.String() + ": the query runs too long (an endless generator such as repeat or range with no end?)")
			}
			r.Err = err
			return r
		}
		if opt.Max > 0 && len(r.Values) == opt.Max {
			r.More = true
			return r
		}
		r.Values = append(r.Values, v)
	}
}

// Hint says what is usually behind an error, or "" when there is nothing useful to add.
func Hint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "expected an object but got: array"):
		return "an array has no fields: go into its elements with .[] (or pick one with .[0]). " +
			"A slice like .[1:] is still an array, so it needs [] too: .[1:][].name"
	case strings.HasPrefix(msg, "expected an object but got: null"),
		strings.HasPrefix(msg, "cannot iterate over: null"):
		return "the value here is null, usually a field that isn't there: check the spelling, " +
			"or add ? to skip it (.items[]?)"
	case strings.HasPrefix(msg, "expected an object but got: string"):
		return "a string has no fields: is the field one level up? tojson / fromjson convert " +
			"between a string and JSON"
	case strings.HasPrefix(msg, "expected an array but got: object"):
		return "an object is indexed by name (.name or .[\"name\"]); .[] goes through its values, " +
			"to_entries gives key/value pairs"
	case strings.Contains(msg, "cannot iterate over: number"),
		strings.Contains(msg, "cannot iterate over: string"):
		return ".[] only goes into arrays and objects; for a number range(n) counts, for a string " +
			"split(\"\") or explode gives the characters"
	}
	return ""
}

var spaceAfterDot = regexp.MustCompile(`(^|[^.])\.\s+$`)

// ParseHint says what is behind a parse error when the expression shows it, or "". So far:
// a space after the dot of a field (`. age`, `.people. name`), which jq reads as `.` and then
// a stray word; and \( in a regex (`sub("\(.*"; "")`), which jq reads as an interpolation.
func ParseHint(expr string, err error) string {
	var pe *gojq.ParseError
	if !errors.As(err, &pe) {
		return ""
	}
	if regexInterp.MatchString(expr) {
		return `\( inside a string starts interpolation ("\(.name)"); for a literal ( in a regex, write \\(`
	}
	at := pe.Offset - len(pe.Token)
	if at < 0 || at > len(expr) || !isIdent(pe.Token) || !spaceAfterDot.MatchString(expr[:at]) {
		return ""
	}
	return fmt.Sprintf("a field is .%s, with no space after the dot", pe.Token)
}

// regexInterp is a regex function's pattern with \( in it: `sub("\(.*"; "")` meant a literal
// paren, but jq reads \( as the start of an interpolation, and the parse fails further on.
var regexInterp = regexp.MustCompile(`\b(test|match|capture|scan|splits?|sub|gsub)\(\s*"[^"]*\\\(`)

func isIdent(s string) bool {
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}

// Caret points at where a parse error happened, under the expression:
//
//	.[1:.people
//	       ^
//
// It gives "" for any other error.
func Caret(expr string, err error) string {
	var pe *gojq.ParseError
	if !errors.As(err, &pe) {
		return ""
	}
	at := pe.Offset - len(pe.Token)
	if at < 0 {
		at = 0
	}
	if at > len(expr) {
		at = len(expr)
	}
	return expr + "\n" + strings.Repeat(" ", len([]rune(expr[:at]))) + "^"
}
