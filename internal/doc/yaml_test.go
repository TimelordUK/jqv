package doc

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestYAMLFixtures translates each testdata/yaml/<name>.yml and compares the JSON, written in
// document order, with <name>.json; `go test ./internal/doc -update` re-blesses. One fixture
// per awkward case (DESIGN §12).
func TestYAMLFixtures(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "yaml", "*.yml"))
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			n, err := ParseYAML(bytes.NewReader(src))
			if err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			n.WriteJSON(&b, "  ")
			golden := strings.TrimSuffix(f, ".yml") + ".json"
			if *update {
				if err := os.WriteFile(golden, b.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to write it)", err)
			}
			if w := strings.ReplaceAll(string(want), "\r\n", "\n"); b.String() != w {
				t.Errorf("differs from %s:\n--- got\n%s--- want\n%s", golden, b.String(), w)
			}
			// The JSON written must read back as the same tree.
			back, err := ParseBytes(b.Bytes())
			if err != nil {
				t.Fatalf("written JSON does not parse: %v", err)
			}
			var again bytes.Buffer
			back.WriteJSON(&again, "  ")
			if again.String() != b.String() {
				t.Errorf("round trip changed it:\n%s", again.String())
			}
		})
	}
}

func TestYAMLSeqAndPaths(t *testing.T) {
	n, err := ParseYAML(strings.NewReader("a:\n  - x: 1\n  - x: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	last := n.Children[0].Children[1].Children[0]
	if got := last.Path(); got != ".a[1].x" {
		t.Errorf("path = %q", got)
	}
	seq := -1
	n.Walk(func(c *Node) bool {
		if c.Seq != seq+1 {
			t.Errorf("Seq %d after %d", c.Seq, seq)
		}
		seq = c.Seq
		return true
	})
}

func TestYAMLErrors(t *testing.T) {
	for _, s := range []string{
		"",                        // nothing at all
		"a: [1, 2",                // unclosed
		"? [1, 2]\n: x\n",         // a list as a key
		"a: &x 1\nb:\n  <<: *x\n", // merging a scalar
		"a: b: c",                 // not YAML
	} {
		if _, err := ParseYAML(strings.NewReader(s)); err == nil {
			t.Errorf("ParseYAML(%q): want error", s)
		}
	}
}

// A few lines of anchors can name a billion values; the expansion is bounded.
func TestYAMLAliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 9; i++ {
		b.WriteString("a" + string(rune('0'+i)) + ": &a" + string(rune('0'+i)) + " [")
		for j := 0; j < 10; j++ {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString("*a" + string(rune('0'+i-1)))
		}
		b.WriteString("]\n")
	}
	_, err := ParseYAML(strings.NewReader(b.String()))
	if err == nil || !strings.Contains(err.Error(), "aliases expand") {
		t.Fatalf("want the alias bound, got %v", err)
	}
}
