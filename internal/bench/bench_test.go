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
			s := New(root.Value())
			s.Doc = root
			got := Transcript(s, lines[1:])
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

// TestLoad: a loaded program goes in front of every line. A file of only defs leaves . the
// document; a main makes . what it gives, and its variables reach the line.
func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	s := New(map[string]any{"a": 1.0})
	s.Do(":load " + write("lib.jq", "def two: 2; # a comment to the end of the line"))
	if got := s.Do("two + .a").Text; got != "3\n" {
		t.Errorf("a def from a file of defs: %q", got)
	}
	if got := s.Program("$root"); got != ". as $root | def two: 2; # a comment to the end of the line\n$root" {
		t.Errorf("program = %q", got)
	}
	s.Do(":load " + write("main.jq", ".a as $x | {b: $x} # main\n"))
	if got := s.Do(".b + $x").Text; got != "2\n" {
		t.Errorf("after a main: %q", got)
	}
	if got := s.Do("two").Text; !strings.Contains(got, "not defined: two/0") {
		t.Errorf("the last file loaded replaces the one before: %q", got)
	}
	s.Do(":arg root E1")
	if got := s.Do("$root").Text; got != "\"E1\"\n" {
		t.Errorf("an argument called root wins: %q", got)
	}
}

// TestEdit: :e puts a unit in the editor, the next line puts it back and changes nothing
// else in the file, a line of defs replaces or adds one, and :w writes it. On a copy of
// team-report.
func TestEdit(t *testing.T) {
	dir := t.TempDir()
	copyFile := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "examples", "jq", "team-report", name))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	path := copyFile("query.jq")
	copyFile("case.json")
	root, err := doc.ParseBytes(mustRead(t, filepath.Join("..", "..", "examples", "jq", "team-report", "input.json")))
	if err != nil {
		t.Fatal(err)
	}
	orig := string(mustRead(t, path))
	s := New(root.Value())
	s.Do(":load " + path)

	r := s.Do(":e avg")
	wantEd := "# The average of f over an array, or null for an empty one (rather than dividing by zero).\n" +
		"def avg(f): if length == 0 then null else (map(f) | add) / length end;"
	if r.Edit != wantEd || s.Prompt() != "edit avg ❯ " {
		t.Fatalf(":e avg gave %q, prompt %q", r.Edit, s.Prompt())
	}
	edited := "# The average of f, rounded.\ndef avg(f):\n  if length == 0 then null\n  else (map(f) | add) / length | round end;"
	r = s.Do(edited)
	if !strings.HasPrefix(r.Text, "put back: avg\n") || !strings.Contains(s.Prompt(), "query.jq* ❯") {
		t.Fatalf("put back: %q, prompt %q", r.Text, s.Prompt())
	}
	if got, want := s.prog.prog.Text, strings.Replace(orig, wantEd, edited, 1); got != want {
		t.Errorf("the program after the edit:\n%s", got)
	}
	if got := s.Do(".avg_age").Text; got != "33\n" {
		t.Errorf("main runs the new avg: %q", got)
	}

	// a mistake goes back to the editor; an empty line then leaves the program as it was
	s.Do(":e $over")
	r = s.Do("| ($all | map(older($min)) as $over")
	if !r.Failed || r.Edit == "" || s.Editing() != "$over" {
		t.Fatalf("a put back that doesn't parse: %+v", r)
	}
	if r = s.Do(""); r.Text != "not changed: $over\n" || s.Editing() != "" {
		t.Fatalf("an empty line: %q", r.Text)
	}

	// a def typed in replaces the one of its name and arity, keeping its doc; a new one is added
	before := s.prog.prog.Text
	r = s.Do("def older($age): select(.age >= $age);")
	if !strings.HasPrefix(r.Text, "replaced: older($age)\n") ||
		s.prog.prog.Text != strings.Replace(before, "select(.age > $age)", "select(.age >= $age)", 1) {
		t.Errorf("replacing older: %q\n%s", r.Text, s.prog.prog.Text)
	}
	r = s.Do("def names: map(.name);")
	if !strings.HasPrefix(r.Text, "added: names\n") || !strings.Contains(s.prog.prog.Text, "def names: map(.name);\n\n# ---- the program") {
		t.Errorf("adding names: %q\n%s", r.Text, s.prog.prog.Text)
	}

	if r = s.Do(":q"); r.Quit || !strings.Contains(r.Text, "unsaved changes") {
		t.Errorf(":q with unsaved changes: %+v", r)
	}
	if r = s.Do(":w " + filepath.Join(dir, "case.json")); !strings.Contains(r.Text, "is there already") {
		t.Errorf(":w over another file: %q", r.Text)
	}
	s.Do(":w")
	if got := string(mustRead(t, path)); got != s.prog.prog.Text || s.Unsaved() {
		t.Errorf(":w wrote\n%s", got)
	}
	if s.Do(":q").Quit != true {
		t.Error(":q after :w should quit")
	}
}

// TestEditKeepsCRLF: what is put back in a file whose lines end in \r\n ends its lines so too.
func TestEditKeepsCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.jq")
	if err := os.WriteFile(path, []byte("# f\r\ndef f: 1;\r\n\r\nf\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(nil)
	s.Do(":load " + path)
	if r := s.Do(":e f"); r.Edit != "# f\ndef f: 1;" {
		t.Fatalf(":e f gave %q", r.Edit)
	}
	s.Do("# f, two\ndef f: 2;")
	if got := s.prog.prog.Text; got != "# f, two\r\ndef f: 2;\r\n\r\nf\r\n" {
		t.Errorf("program %q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
