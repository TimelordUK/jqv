package bench

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/config"
	"github.com/TimelordUK/jqv/internal/doc"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestSessions runs each testdata/<name>.session and compares the transcript with
// <name>.golden; `go test ./internal/bench -update` re-blesses. A session's first line is
// `input: file.json`, a file in the repository's testdata; the rest are typed lines.
func TestSessions(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.session"))
	if len(files) == 0 {
		t.Fatal("no sessions")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".session")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n"), "\n")
			in, ok := strings.CutPrefix(lines[0], "input: ")
			if !ok {
				t.Fatalf("%s: the first line must be `input: file.json`", f)
			}
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", in))
			if err != nil {
				t.Fatal(err)
			}
			root, err := doc.ParseBytes(b)
			if err != nil {
				t.Fatal(err)
			}
			got := Transcript(New(root.Value()), lines[1:])
			golden := filepath.Join("testdata", name+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to write it)", err)
			}
			if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
				t.Errorf("transcript differs from %s:\n--- got\n%s--- want\n%s", golden, got, w)
			}
		})
	}
}

func TestQuitStopsTheSession(t *testing.T) {
	s := New(nil)
	if !s.Do(":q").Quit {
		t.Error(":q should quit")
	}
	if s.Do("1").Quit {
		t.Error("a line of jq should not quit")
	}
}

func TestProgramBindsRootOnlyWhenUsed(t *testing.T) {
	s := New(map[string]any{"a": []any{1.0}})
	s.Do(":push .a")
	if got := s.Program(""); got != ".a" {
		t.Errorf("program = %q", got)
	}
	s.Do(":push .[] | select(. < ($root.a | length))")
	if got, want := s.Program(""), ". as $root | .a | .[] | select(. < ($root.a | length))"; got != want {
		t.Errorf("program = %q, want %q", got, want)
	}
}

func TestConfiguredPipe(t *testing.T) {
	s := New(map[string]any{"a": 1.0})
	if r := s.Do(":!"); r.Pipe != "" || !strings.Contains(r.Text, "[repl]") {
		t.Fatalf("with no pipe set, a bare :! should say how to set one: %+v", r)
	}
	probs := s.Configure(config.Parse("config.toml", "[repl]\npipe = \"bat -l json\"\ncolour = 1\n"))
	if len(probs) != 1 || !strings.Contains(probs[0], "colour") {
		t.Errorf("problems = %q", probs)
	}
	s.Do(".a")
	if r := s.Do(":!"); r.Pipe != "bat -l json" || r.Input != "1\n" {
		t.Errorf("bare :! = %+v, want the configured pipe", r)
	}
	if r := s.Do(":! clip"); r.Pipe != "clip" {
		t.Errorf(":! clip = %+v: a command given beats the config", r)
	}
	if p := s.Configure(config.Parse("config.toml", "[repl]\npipe = 3\n")); len(p) != 1 {
		t.Errorf("a pipe that isn't a string: problems %q", p)
	}
}

// TestFailedLines: history keeps every line, and marks the jq that didn't parse or run.
func TestFailedLines(t *testing.T) {
	s := New(map[string]any{"a": []any{1.0, "x"}})
	for _, c := range []struct {
		line   string
		failed bool
	}{
		{".a", false},
		{".a[] + 1", true}, // runs, then fails on "x"
		{".a[", true},      // doesn't parse
		{".nope", false},
		{":push .a", false},
		{":help nothing", false},
	} {
		if r := s.Do(c.line); r.Failed != c.failed {
			t.Errorf("%q: Failed = %v, want %v", c.line, r.Failed, c.failed)
		}
	}
}

// TestLongOutputIsCut (Q-039): `.` on a big document printed every line of it. What a line
// shows stops at Lines, says how much more there is, and :pp still gives the whole value.
func TestLongOutputIsCut(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "FIX4.4.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	s := New(root.Value())
	r := s.Do(".")
	lines := strings.Split(strings.TrimRight(r.Text, "\n"), "\n")
	if len(lines) != s.Lines+1 {
		t.Fatalf(". gave %d lines, want %d and a note", len(lines), s.Lines+1)
	}
	if note := lines[len(lines)-1]; !strings.Contains(note, "more lines") || !strings.Contains(note, ":pp") {
		t.Errorf("note = %q, want how many more lines and how to see them", note)
	}
	if full := s.Do(":pp").Text; strings.Count(full, "\n") < 10*s.Lines {
		t.Errorf(":pp gave %d lines; it should be the whole value", strings.Count(full, "\n"))
	}
	if v := s.Do("$_ | length").Text; v != "1\n" {
		t.Errorf("$_ | length = %q: $_ must be the whole value, not what was shown", v)
	}

	// The 50 one-line outputs the output cap allows all still fit, as before.
	r = s.Do(".fix.fields.field[] | ._name")
	if strings.Contains(r.Text, "more lines") || !strings.Contains(r.Text, "the first 50 outputs") {
		t.Errorf("50 short outputs should show whole, with the outputs note:\n%s", r.Text)
	}
	// Several outputs that are long together are cut as one.
	r = s.Do(".fix.messages.message[0:10][]")
	if !strings.Contains(r.Text, "more lines") || !strings.Contains(r.Text, "(10 outputs)") {
		t.Errorf("10 long outputs: want the cut and the outputs note:\n%s", r.Text)
	}

	// Short output is untouched, and lines = 0 in config.toml turns the cut off.
	if r := s.Do(".fix._major"); r.Text != "\"4\"\n" {
		t.Errorf("short output = %q", r.Text)
	}
	if p := s.Configure(config.Parse("config.toml", "[repl]\nlines = 0\n")); len(p) != 0 {
		t.Fatalf("problems %q", p)
	}
	if n := strings.Count(s.Do(".").Text, "\n"); n < 10*40 {
		t.Errorf("with lines = 0, . gave %d lines; want all of it", n)
	}
	for _, bad := range []string{"lines = -1", "lines = \"ten\""} {
		if p := s.Configure(config.Parse("config.toml", "[repl]\n"+bad+"\n")); len(p) != 1 {
			t.Errorf("%s: problems %q, want one", bad, p)
		}
	}
}

// TestStageHint (Q-041): with a stage pushed, a line written for the whole document fails or
// gives nulls, and "check the spelling" sent the owner the wrong way. When the line works on
// $root, the hint says . is the stages' result and how to reach the document.
func TestStageHint(t *testing.T) {
	root := map[string]any{"fix": map[string]any{"fields": []any{
		map[string]any{"n": "1", "name": "Account"},
		map[string]any{"n": "2", "name": "AdvId"},
	}}}
	s := New(root)
	s.Do(":push INDEX(.fix.fields[]; .n)")
	for _, line := range []string{
		".fix.fields[] | .name", // fails: cannot iterate over null
		".fix",                  // gives null
	} {
		r := s.Do(line).Text
		for _, want := range []string{"stage 1", "object, 2 keys", "$root | " + line, ":pop"} {
			if !strings.Contains(r, want) {
				t.Errorf("%s: want %q in\n%s", line, want, r)
			}
		}
		if strings.Contains(r, "check the spelling") {
			t.Errorf("%s: the spelling hint is the wrong way here:\n%s", line, r)
		}
	}
	// No hint when the line fails on the document too, or when there are no stages.
	if r := s.Do(".nope[]").Text; strings.Contains(r, "$root |") {
		t.Errorf(".nope[] fails on $root too; no stage hint wanted:\n%s", r)
	}
	s.Do(":reset")
	if r := s.Do(".nope[]").Text; strings.Contains(r, "stage") {
		t.Errorf("no stages, no stage hint:\n%s", r)
	}
}

// TestNullNoteKeysCapped (Q-042): the all-null note listed every key, 912 of them on FIX4.4.
func TestNullNoteKeysCapped(t *testing.T) {
	m := map[string]any{}
	for i := range 30 {
		m[fmt.Sprintf("k%02d", i)] = i
	}
	r := New(m).Do(".nope").Text
	if !strings.Contains(r, "k00, k01") || strings.Contains(r, "k29") || !strings.Contains(r, "30 in all") {
		t.Errorf("want the first keys and the count, not all 30:\n%s", r)
	}
	if r := New(map[string]any{"a": 1, "b": 2}).Do(".nope").Text; !strings.Contains(r, "Keys here: a, b)") {
		t.Errorf("a few keys are listed whole:\n%s", r)
	}
}
