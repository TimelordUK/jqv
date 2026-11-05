package filter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/itchyny/gojq"
)

// corpusCase is one [[case]] of testdata/filter/cases.toml.
type corpusCase struct {
	Name    string
	Records string
	Layers  []string
	Keep    []string
	Errors  int
	Reject  string
	Why     string
}

func corpus(t *testing.T) (*doc.Node, []byte, []corpusCase) {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "filter")
	input, err := os.ReadFile(filepath.Join(dir, "trades.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Case []corpusCase }
	md, err := toml.DecodeFile(filepath.Join(dir, "cases.toml"), &file)
	if err != nil {
		t.Fatal(err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		t.Fatalf("cases.toml: unknown fields %v", u)
	}
	return root, input, file.Case
}

// at finds the node whose jq path is path.
func at(root *doc.Node, path string) *doc.Node {
	var found *doc.Node
	root.Walk(func(n *doc.Node) bool {
		if n.Path() == path {
			found = n
		}
		return found == nil
	})
	return found
}

func ids(ns []*doc.Node) []string {
	out := []string{}
	for _, n := range ns {
		for _, c := range n.Children {
			if c.Key == "id" {
				out = append(out, c.Raw)
			}
		}
	}
	return out
}

func TestCorpus(t *testing.T) {
	root, input, cases := corpus(t)
	realJQ, _ := exec.LookPath("jq")
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var layers []Layer
			for _, e := range c.Layers {
				l, err := Compile(e)
				if err != nil {
					if c.Reject == "" || !strings.Contains(err.Error(), c.Reject) {
						t.Fatalf("%s: %v (want reject %q)", e, err, c.Reject)
					}
					return
				}
				layers = append(layers, l)
			}
			if c.Reject != "" {
				t.Fatalf("want the stack refused with %q", c.Reject)
			}
			records := at(root, c.Records)
			if records == nil {
				t.Fatalf("no %s in the input", c.Records)
			}
			r := Apply(records, layers)
			if got := ids(r.Kept); !reflect.DeepEqual(got, c.Keep) {
				t.Errorf("kept %v, want %v", got, c.Keep)
			}
			if r.Errors != c.Errors {
				t.Errorf("errors %d, want %d (first: %v)", r.Errors, c.Errors, r.Err)
			}

			// The same stack as one jq program must leave the same records, and the rest of the
			// document untouched.
			prog := Lift(c.Records, layers) + " | [" + c.Records + "[].id], .source"
			q, err := gojq.Parse(prog)
			if err != nil {
				t.Fatalf("lifted %s: %v", prog, err)
			}
			var got []any
			it := q.Run(root.Value())
			for v, ok := it.Next(); ok; v, ok = it.Next() {
				if err, ok := v.(error); ok {
					t.Fatalf("gojq %s: %v", prog, err)
				}
				got = append(got, v)
			}
			want := []any{toAny(c.Keep), "demo-oms"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("gojq %s\n gave %v, want %v", prog, got, want)
			}
			if realJQ != "" {
				cmd := exec.Command(realJQ, "-c", prog)
				cmd.Stdin = bytes.NewReader(input)
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("jq %s: %v", prog, err)
				}
				wantOut, _ := json.Marshal(c.Keep)
				if lines := strings.Fields(string(out)); len(lines) != 2 || lines[0] != string(wantOut) {
					t.Errorf("jq %s\n gave %s, want %s", prog, out, wantOut)
				}
			}
		})
	}
}

func toAny(ss []string) []any {
	out := []any{}
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func TestEmptyStackKeepsEverything(t *testing.T) {
	root, _, _ := corpus(t)
	if r := Apply(at(root, ".trades"), nil); len(r.Kept) != 8 {
		t.Fatalf("kept %d", len(r.Kept))
	}
	if got := Lift(".trades", nil); got != "." {
		t.Fatalf("lift of nothing: %s", got)
	}
}

func TestFirstErrorSaysWhere(t *testing.T) {
	root, _, _ := corpus(t)
	l, _ := Compile(`.instrument.symbol == "EUR"`)
	r := Apply(at(root, ".trades"), []Layer{l})
	if r.Err == nil || !strings.Contains(r.Err.Error(), ".trades[6]") {
		t.Fatalf("first error %v should name T7's path", r.Err)
	}
}
