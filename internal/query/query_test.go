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
