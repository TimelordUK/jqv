package bench

import (
	"flag"
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
