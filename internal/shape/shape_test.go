package shape

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestOutlines prints the outline of each document in the repository's testdata against
// testdata/<name>.golden; `go test ./internal/shape -update` re-blesses.
func TestOutlines(t *testing.T) {
	for _, name := range []string{"orders", "people", "FIX4.4", "nobel_prize", "users_100", "env", "files", "users.schema"} {
		t.Run(name, func(t *testing.T) {
			got := Outline(FromNode(load(t, name)), Options{})
			golden(t, name, got)
		})
	}
}

func TestDepth(t *testing.T) {
	golden(t, "FIX4.4-depth2", Outline(FromNode(load(t, "FIX4.4")), Options{Depth: 2}))
}

// Values and nodes give the same shape; with the document's key order, in the same order.
func TestValuesMatchNodes(t *testing.T) {
	for _, name := range []string{"orders", "nobel_prize"} {
		n := load(t, name)
		lines := func(s string) string {
			l := strings.Split(s, "\n")
			sort.Strings(l)
			return strings.Join(l, "\n")
		}
		a := Outline(FromValues([]any{n.Value()}, nil), Options{})
		b := Outline(FromNode(n), Options{})
		if lines(a) != lines(b) {
			t.Errorf("%s: values and nodes differ:\n%s\n---\n%s", name, a, b)
		}
		if a := Outline(FromValues([]any{n.Value()}, KeyOrder(n)), Options{}); a != b {
			t.Errorf("%s: values in the document's key order differ:\n%s\n---\n%s", name, a, b)
		}
	}
}

func TestFind(t *testing.T) {
	s := FromNode(load(t, "FIX4.4"))
	f := s.Find("fix", "messages", "message", "[]", "_name")
	if f == nil {
		t.Fatal("no .fix.messages.message[]._name")
	}
	if d, _ := f.Distinct(); d != f.Count || f.Count < 90 {
		t.Errorf("message names: %d distinct of %d", d, f.Count)
	}
	if s.Find("fix", "nope") != nil {
		t.Error("found a key that isn't there")
	}
}

func TestMixed(t *testing.T) {
	s := FromValues([]any{
		map[string]any{"a": []any{1, 2}},
		map[string]any{"a": map[string]any{"b": 1}},
		map[string]any{"a": nil},
	}, nil)
	a := s.Find("a")
	if !a.Mixed() {
		t.Errorf("array and object at .a should be mixed: %v", a.Kinds)
	}
	if b := FromValues([]any{"x", nil}, nil); b.Mixed() {
		t.Error("a string or null is optional, not mixed")
	}
}

func load(t *testing.T, name string) *doc.Node {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("outline differs from %s:\n--- got\n%s--- want\n%s", path, got, w)
	}
}

// An object that passed through jq unchanged keeps its keys as written, even when a key
// first appears elsewhere earlier: FIX's fields have _number before _name, but the header's
// fields, earlier in the file, have _name alone.
func TestKeyOrderExactSets(t *testing.T) {
	n := load(t, "FIX4.4")
	field := n.Value().(map[string]any)["fix"].(map[string]any)["fields"].(map[string]any)["field"].([]any)[0]
	s := FromValues([]any{field}, KeyOrder(n))
	var got []string
	for _, c := range s.Children {
		got = append(got, c.Key)
	}
	if want := "_number _name _type"; strings.Join(got, " ") != want {
		t.Errorf("keys %v, want %s", got, want)
	}
	built := FromValues([]any{map[string]any{"zz": 1, "_name": "x"}}, KeyOrder(n))
	if built.Children[0].Key != "_name" {
		t.Errorf("a built object's known keys should come before unknown ones: %s first", built.Children[0].Key)
	}
}
