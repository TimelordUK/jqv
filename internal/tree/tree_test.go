package tree

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
)

var update = flag.Bool("update", false, "rewrite golden files")

const sample = `{"name":"jqv","meta":{"v":1,"tags":["a","b"]},"items":[{"qty":2,"note":"x\"y"},{"qty":null}],"empty":{}}`

func newTree(t *testing.T, s string) *Tree {
	t.Helper()
	root, err := doc.ParseBytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return New(root)
}

// render is the tree as plain text, two spaces per depth, as golden files store it.
func render(t *Tree) string {
	var b strings.Builder
	for _, r := range t.Rows() {
		b.WriteString(strings.Repeat("  ", r.Depth))
		b.WriteString(t.Label(r))
		b.WriteString("\n")
	}
	return b.String()
}

// golden compares got with testdata/<name>.golden; `go test ./internal/tree -update` re-blesses.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("%s differs\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

func TestOpensFirstLevel(t *testing.T) {
	golden(t, "first_level", render(newTree(t, sample)))
}

func TestExpandAll(t *testing.T) {
	tr := newTree(t, sample)
	tr.ExpandAll(tr.Root)
	golden(t, "expand_all", render(tr))
}

func TestCollapseAllKeepsRootOpen(t *testing.T) {
	tr := newTree(t, sample)
	tr.ExpandAll(tr.Root)
	tr.CollapseAll(tr.Root)
	if got := render(newTree(t, sample)); render(tr) != got {
		t.Fatalf("collapse all should return to the first level\n%s", render(tr))
	}
}

func TestToggleAndIndexOf(t *testing.T) {
	tr := newTree(t, sample)
	items := tr.Root.Children[2]
	if tr.Len() != 5 {
		t.Fatalf("rows = %d, want 5", tr.Len())
	}
	tr.Toggle(items)
	if tr.Len() != 7 || !tr.Expanded(items) {
		t.Fatalf("after open: rows = %d", tr.Len())
	}
	deep := items.Children[0].Children[0]
	if tr.IndexOf(deep) != -1 {
		t.Fatal("qty inside a closed element should be hidden")
	}
	tr.Toggle(items)
	if tr.Len() != 5 {
		t.Fatalf("after close: rows = %d", tr.Len())
	}
}

func TestToggleScalarIsNoop(t *testing.T) {
	tr := newTree(t, sample)
	tr.Toggle(tr.Root.Children[0])
	if tr.Len() != 5 {
		t.Fatalf("rows = %d", tr.Len())
	}
}

func TestCollapseBelowKeepsNodeOpen(t *testing.T) {
	tr := newTree(t, sample)
	tr.ExpandAll(tr.Root)
	items := tr.Root.Children[2]
	tr.CollapseBelow(items)
	if !tr.Expanded(items) || tr.Expanded(items.Children[0]) || !tr.Expanded(tr.Root.Children[1]) {
		t.Fatal("want items open, its elements closed, siblings untouched")
	}
	tr.CollapseBelow(tr.Root.Children[0]) // a scalar: nothing to do, no panic
}

func TestRevealOpensOnlyAncestors(t *testing.T) {
	tr := newTree(t, sample)
	note := tr.Root.Children[2].Children[0].Children[1] // .items[0].note
	i := tr.Reveal(note)
	if i < 0 || tr.Rows()[i].Node != note {
		t.Fatalf("Reveal = %d", i)
	}
	if tr.Expanded(tr.Root.Children[1]) || tr.Expanded(tr.Root.Children[2].Children[1]) {
		t.Fatal("Reveal opened something that is not an ancestor")
	}
}

func TestShowOnly(t *testing.T) {
	tr := newTree(t, `[{"name":"Olivia","friends":[{"name":"Bob"},{"name":"olivia"}]},{"name":"Ann","friends":[{"name":"Olivia"}]},{"name":"Zed","friends":[{"name":"Bob"}]}]`)
	tr.ExpandAll(tr.Root)
	u := tr.Root.Children
	tr.ShowOnly([]*doc.Node{
		u[0].Children[0],                         // .[0].name
		u[0].Children[1].Children[1].Children[0], // .[0].friends[1].name
		u[1].Children[1].Children[0].Children[0], // .[1].friends[0].name
	})
	golden(t, "show_only", render(tr))
}

func TestExpandToDepth(t *testing.T) {
	tr := newTree(t, sample)
	tr.ExpandToDepth(2)
	golden(t, "depth_2", render(tr))
}
