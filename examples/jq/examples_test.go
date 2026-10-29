// Package examples runs every jq sample under examples/jq through gojq, the engine jqv
// embeds, and checks the output against expected.jsonl, which is produced with real jq.
// So a sample that stops working fails here, and so does any gojq/jq difference it hits.
package examples

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/itchyny/gojq"
)

// sample is case.json: --arg values and whether jq runs with -n.
type sample struct {
	Args      map[string]string `json:"args"`
	NullInput bool              `json:"null_input"`
}

// values decodes every JSON value in a file (one document, or JSON Lines).
func values(t *testing.T, path string) []any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []any
	d := json.NewDecoder(f)
	for {
		var v any
		if err := d.Decode(&v); err == io.EOF {
			return out
		} else if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, v)
	}
}

// normal round-trips through JSON so gojq's ints and encoding/json's float64s compare equal.
func normal(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var n any
	json.Unmarshal(b, &n)
	return n
}

func TestSamples(t *testing.T) {
	dirs, _ := filepath.Glob("*/query.jq")
	if len(dirs) == 0 {
		t.Fatal("no samples found")
	}
	for _, qpath := range dirs {
		dir := filepath.Dir(qpath)
		t.Run(dir, func(t *testing.T) {
			src, _ := os.ReadFile(qpath)
			var c sample
			if b, err := os.ReadFile(filepath.Join(dir, "case.json")); err == nil {
				if err := json.Unmarshal(b, &c); err != nil {
					t.Fatalf("case.json: %v", err)
				}
			}
			inputs, _ := filepath.Glob(filepath.Join(dir, "input.json*"))
			if len(inputs) != 1 {
				t.Fatalf("want one input.json or input.jsonl, found %v", inputs)
			}
			in := values(t, inputs[0])

			q, err := gojq.Parse(string(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var names []string
			for k := range c.Args {
				names = append(names, k)
			}
			sort.Strings(names)
			var vars []string
			var vals []any
			for _, k := range names {
				vars, vals = append(vars, "$"+k), append(vals, c.Args[k])
			}
			opts := []gojq.CompilerOption{gojq.WithVariables(vars)}
			if c.NullInput {
				opts = append(opts, gojq.WithInputIter(gojq.NewIter(in...)))
				in = []any{nil}
			}
			code, err := gojq.Compile(q, opts...)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			var got []any
			for _, v := range in {
				iter := code.Run(v, vals...)
				for {
					r, ok := iter.Next()
					if !ok {
						break
					}
					if err, ok := r.(error); ok {
						t.Fatalf("run: %v", err)
					}
					got = append(got, normal(t, r))
				}
			}
			want := values(t, filepath.Join(dir, "expected.jsonl"))
			if !reflect.DeepEqual(got, want) {
				g, _ := json.MarshalIndent(got, "", " ")
				w, _ := json.MarshalIndent(want, "", " ")
				t.Fatalf("output differs\n--- got ---\n%s\n--- want (real jq) ---\n%s", g, w)
			}
		})
	}
}
