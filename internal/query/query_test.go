package query

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
)

func TestRunGivesOutputsWithVariables(t *testing.T) {
	r := Run(`.[] | . + $n`, []any{1, 2}, map[string]any{"$n": 10}, Options{})
	if r.Err != nil || len(r.Values) != 2 || r.Values[0] != 11 || r.Values[1] != 12 {
		t.Errorf("got %+v", r)
	}
}

func TestMaxStopsAnEndlessGenerator(t *testing.T) {
	r := Run(`repeat(1)`, nil, nil, Options{Max: 5})
	if r.Err != nil || len(r.Values) != 5 || !r.More {
		t.Errorf("got %d values, more %v, err %v", len(r.Values), r.More, r.Err)
	}
	r = Run(`range(5)`, nil, nil, Options{Max: 5})
	if r.More {
		t.Error("exactly Max outputs is not more")
	}
}

func TestTimeoutStopsALongRun(t *testing.T) {
	start := time.Now()
	r := Run(`last(range(1e12))`, nil, nil, Options{Timeout: 50 * time.Millisecond})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "too long") {
		t.Errorf("err = %v", r.Err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

func TestErrorsKeepWhatCameBefore(t *testing.T) {
	r := Run(`.[] | .a`, []any{map[string]any{"a": 1}, "x"}, nil, Options{})
	if len(r.Values) != 1 || r.Err == nil {
		t.Errorf("got %+v", r)
	}
}

func TestHints(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{`.[1:].people`, "A slice like .[1:] is still an array"},
		{`.[0].missing[]`, "add ?"},
		{`.[0].name.first`, "a string has no fields"},
		{`.[0][0]`, "an object is indexed by name"},
		{`.[0].name | .[]`, ".[] only goes into arrays and objects"},
	} {
		r := Run(c.expr, []any{map[string]any{"name": "Ann"}}, nil, Options{})
		if h := Hint(r.Err); !strings.Contains(h, c.want) {
			t.Errorf("%s: err %v, hint %q, want it to say %q", c.expr, r.Err, h, c.want)
		}
	}
	if Hint(errors.New("something else")) != "" {
		t.Error("an unknown error should have no hint")
	}
}

func TestCaretPointsAtTheBadToken(t *testing.T) {
	expr := ".a | .b c"
	_, err := gojq.Parse(expr)
	if got, want := Caret(expr, err), ".a | .b c\n        ^"; got != want {
		t.Errorf("caret:\n%s\nwant:\n%s", got, want)
	}
	if Caret(expr, errors.New("not a parse error")) != "" {
		t.Error("only parse errors get a caret")
	}
}

func TestParseHintSpaceAfterDot(t *testing.T) {
	for expr, want := range map[string]string{
		`.[].people[] | select(. age > 30)`: "a field is .age, with no space",
		`.people. name`:                     "a field is .name, with no space",
	} {
		_, err := gojq.Parse(expr)
		if h := ParseHint(expr, err); !strings.Contains(h, want) {
			t.Errorf("%s: err %v, hint %q, want it to say %q", expr, err, h, want)
		}
	}
	for _, expr := range []string{`. as $x | $x`, `.a b`, `. and .b c`} { // valid after the dot, or another mistake
		_, err := gojq.Parse(expr)
		if h := ParseHint(expr, err); h != "" {
			t.Errorf("%s: err %v, want no hint, got %q", expr, err, h)
		}
	}
}

// TestBindingHint is from daily use (2026-11-08): after `X as $i |`, . is still what came
// in, not X, so a map over it ran on the document.
func TestBindingHint(t *testing.T) {
	failed := errors.New("cannot add: object and number")
	for _, expr := range []string{
		`[range(6)] as $i | map(. +2)`,
		`[range(6)] as $i | map($i +2)`,
		`.a | [.[] | .b] as $bs | map(. * 2)`,
	} {
		if h := BindingHint(expr, failed); !strings.Contains(h, "pipe it in") {
			t.Errorf("%s: hint %q, want the binding hint", expr, h)
		}
	}
	for _, expr := range []string{
		`[range(6)] as $i | $i | map(. + 2)`, // starts from $i: something else is wrong
		`map(. + 2)`,                         // no binding
		`reduce .[] as $x (0; . + $x)`,       // reduce's binding has no |
	} {
		if h := BindingHint(expr, failed); h != "" {
			t.Errorf("%s: want no hint, got %q", expr, h)
		}
	}
	_, perr := gojq.Parse(`[range(6)] as $i | map(`)
	if h := BindingHint(`[range(6)] as $i | map(`, perr); h != "" {
		t.Errorf("a parse error isn't this mistake: %q", h)
	}
}

func TestParseHintInterpolationInRegex(t *testing.T) {
	for _, expr := range []string{`.name | sub("\(.*"; "")`, `select(.name | test("^\(A"))`} {
		_, err := gojq.Parse(expr)
		if h := ParseHint(expr, err); !strings.Contains(h, `write \\(`) {
			t.Errorf("%s: err %v, hint %q, want the interpolation hint", expr, err, h)
		}
	}
	for _, expr := range []string{`"\(.a) b" c`, `test("a") b`} { // interpolation outside a regex, or another mistake
		_, err := gojq.Parse(expr)
		if h := ParseHint(expr, err); h != "" {
			t.Errorf("%s: err %v, want no hint, got %q", expr, err, h)
		}
	}
}

func TestBindingHintUsesTheLine(t *testing.T) {
	h := BindingHint(`.a | [.[] | .b] as $bs | map(. * 2)`, errors.New("cannot multiply"))
	if !strings.Contains(h, "`[.[] | .b] | map(. * 2)`") || !strings.Contains(h, "`[.[] | .b] as $bs | $bs | map(. * 2)`") {
		t.Errorf("the hint should show the line's own X and body: %q", h)
	}
}
