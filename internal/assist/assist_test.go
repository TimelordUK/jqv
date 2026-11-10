package assist

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/bench"
)

func session(t *testing.T) *bench.Session {
	t.Helper()
	b, err := os.ReadFile("../../testdata/people.json")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return bench.New(v)
}

func TestHelp(t *testing.T) {
	h := HelpFor(session(t))
	if out, cur := h(".[] | ltrim", 9); !strings.HasPrefix(out, "trim, ltrim, rtrim") || cur != 9 {
		t.Errorf("F1 on ltrim: %q, cursor %d", out, cur)
	}
	if out, _ := h("@csv", 1); !strings.Contains(out, "Format strings") {
		t.Errorf("F1 on @csv: %q", out)
	}
	if out, _ := h(". | ", 4); !strings.Contains(out, ":push") {
		t.Errorf("F1 on no word is the REPL's help: %q", out)
	}
}

// The line from daily use: the reduce fails, and F5 on select shows what reaches it.
const failing = `.[].people[] | select(.age > 30) | reduce .[] as $item (0; . + $item)`

func TestRunToCursor(t *testing.T) {
	s := session(t)
	r := RunToCursor(s)
	full := s.Do(failing).Text
	if !strings.Contains(full, "cannot add") {
		t.Skipf("people.json no longer makes the reduce fail: %q", full)
	}
	cur := strings.Index(failing, "select") + 2
	out, to := r(failing, cur)
	if to != cur {
		t.Errorf("F5 leaves the cursor: %d", to)
	}
	if !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") || strings.Contains(out, "cannot add") {
		t.Errorf("F5 on select runs up to it:\n%s", out)
	}
	if out, _ := r(":push "+failing, 6+cur); !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") {
		t.Errorf("F5 in :push's argument:\n%s", out)
	}
	if out, _ := r(":stages", 3); out != notJQ {
		t.Errorf("F5 on a command: %q", out)
	}
}

func TestStep(t *testing.T) {
	s := session(t)
	back, on := Step(s, -1), Step(s, +1)
	var seen []string
	cur := 0
	for range 5 {
		out, to := on(failing, cur)
		if out == "" {
			break
		}
		seen = append(seen, strings.SplitN(out, "\n", 2)[0])
		cur = to
	}
	want := []string{
		"▸ .[].people[]",
		"▸ .[].people[] | select(.age > 30)",
		"▸ " + failing,
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Errorf("ctrl+down along the line:\n got  %q\n want %q", seen, want)
	}
	if out, to := on(failing, cur); out != "" || to != cur {
		t.Errorf("past the last stage it stays: %q %d", out, to)
	}
	out, to := back(failing, cur)
	if !strings.HasPrefix(out, "▸ .[].people[] | select(.age > 30)\n") || to != strings.Index(failing, " | reduce") {
		t.Errorf("ctrl+up: %q at %d", out, to)
	}
}

func TestLayoutLine(t *testing.T) {
	s := session(t)
	f8 := LayoutLine(s)
	if out, cur := f8("[.a | .b]", 3); out != "[\n    .a\n    | .b\n]\n" || cur != 3 {
		t.Errorf("F8: %q, cursor %d", out, cur)
	}
	if out, _ := f8(":push .a|.b", 0); out != ".a\n| .b\n" {
		t.Errorf("F8 on :push lays out its jq: %q", out)
	}
	if out, _ := f8("", 0); !strings.HasPrefix(out, "nothing to show") {
		t.Errorf("F8 on nothing run yet: %q", out)
	}
	s.Do(".[] | .team")
	if out, _ := f8("", 0); out != ".[]\n| .team\n" {
		t.Errorf("F8 on an empty line lays out the last line: %q", out)
	}
}
