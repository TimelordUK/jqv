package catalog

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
)

func mustLoad(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// run gives what gojq gives for an example, or its error.
func run(x Example) ([]any, error) {
	q, err := gojq.Parse(x.Program)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(q, gojq.WithEnvironLoader(os.Environ))
	if err != nil {
		return nil, err
	}
	in, _, err := x.Values()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out []any
	it := code.RunWithContext(ctx, in)
	for {
		v, ok := it.Next()
		if !ok {
			return out, nil
		}
		if e, ok := v.(error); ok {
			return out, e
		}
		out = append(out, v)
	}
}

// TestExamples runs every example in the manual through gojq. Those that don't give what the
// manual says must be in Differs, and everything in Differs must still differ.
func TestExamples(t *testing.T) {
	t.Setenv("PAGER", "less") // the $ENV examples, as jq's own tests run them
	c := mustLoad(t)
	n, differ := 0, map[string]bool{}
	for _, e := range c.Entries {
		for _, x := range e.Examples {
			n++
			got, err := run(x)
			ok := err == nil && x.Same(got)
			if !ok {
				differ[x.Program] = true
			}
			switch _, listed := Differs[x.Program]; {
			case !ok && !listed:
				t.Errorf("%s: %q on %s\n gojq gives %v %v, the manual %v\n (add it to Differs if gojq is right to)",
					Plain(e.Title), x.Program, x.Input, got, err, x.Output)
			case ok && listed:
				t.Errorf("%s: %q now works under gojq: take it out of Differs", Plain(e.Title), x.Program)
			}
		}
	}
	for p := range Differs {
		if !differ[p] {
			if _, found := find(c, p); !found {
				t.Errorf("Differs lists %q, which isn't an example in the manual", p)
			}
		}
	}
	if n < 200 {
		t.Errorf("only %d examples: is manual.yml whole?", n)
	}
}

func find(c *Catalog, program string) (Example, bool) {
	for _, e := range c.Entries {
		for _, x := range e.Examples {
			if x.Program == program {
				return x, true
			}
		}
	}
	return Example{}, false
}

// TestExamplesUnderJQ runs every example through real jq when it is on the PATH (or $JQ
// names it): the manual must be right about the jq it came from.
func TestExamplesUnderJQ(t *testing.T) {
	jq := os.Getenv("JQ")
	if jq == "" {
		jq, _ = exec.LookPath("jq")
	}
	if jq == "" {
		t.Skip("jq not on the PATH (set JQ to its path to run this)")
	}
	t.Setenv("PAGER", "less")
	c := mustLoad(t)
	for _, e := range c.Entries {
		for _, x := range e.Examples {
			cmd := exec.Command(jq, "-c", x.Program)
			cmd.Stdin = strings.NewReader(x.Input)
			b, err := cmd.Output()
			if err != nil {
				t.Errorf("%s: %q: jq: %v", Plain(e.Title), x.Program, err)
				continue
			}
			var got []any
			for _, l := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n")), "\n") {
				if l == "" {
					continue
				}
				v, err := decode(l)
				if err != nil {
					t.Fatalf("%q: jq gave %q: %v", x.Program, l, err)
				}
				got = append(got, v)
			}
			if !x.Same(got) {
				t.Errorf("%s: %q: jq gives %s, the manual %v", Plain(e.Title), x.Program, b, x.Output)
			}
		}
	}
}

// TestNamesAgainstGojq keeps NotInGojq true both ways. Operators and keywords (`|`, `reduce`,
// `$ENV`) aren't in `builtins` and aren't checked.
func TestNamesAgainstGojq(t *testing.T) {
	c := mustLoad(t)
	r, err := run(Example{Program: `[builtins[] | split("/")[0]] | unique`, Input: "null"})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, v := range r[0].([]any) {
		have[v.(string)] = true
	}
	var missing []string
	for _, n := range c.Names() {
		if leadName.MatchString(n) && !strings.HasPrefix(n, "$") && !have[n] && !keywords[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	var want []string
	for n := range NotInGojq {
		want = append(want, n)
	}
	sort.Strings(want)
	if strings.Join(missing, " ") != strings.Join(want, " ") {
		t.Errorf("names in the manual that gojq doesn't define:\n got       %v\n NotInGojq %v", missing, want)
	}
}

func TestLookup(t *testing.T) {
	c := mustLoad(t)
	for q, want := range map[string]string{
		"trim":                "`trim`, `ltrim`, `rtrim`",
		"rtrim":               "`trim`, `ltrim`, `rtrim`",
		"map_values":          "`map(f)`, `map_values(f)`",
		"|":                   "Pipe: `|`",
		"?//":                 "Destructuring Alternative Operator: `?//`",
		"as":                  "Variable / Symbolic Binding Operator: `... as $identifier | ...`",
		"$ENV":                "`$ENV`, `env`",
		"math":                "Math",
		"Regular Expressions": "Regular expressions",
	} {
		es, exact := c.Lookup(q)
		if !exact || len(es) == 0 || es[0].Title != want {
			var got []string
			for _, e := range es {
				got = append(got, e.Title)
			}
			t.Errorf("Lookup(%q) = %v %v, want %q", q, got, exact, want)
		}
	}
	if es, _ := c.Lookup("splits"); len(es) != 1 {
		t.Errorf("splits: one entry, however many forms its title names; got %d", len(es))
	}
	if es, _ := c.Lookup("first"); len(es) != 2 {
		t.Errorf("first: want both entries (first(expr) and first), got %d", len(es))
	}
	if es, exact := c.Lookup("whitespace"); exact || len(es) == 0 {
		t.Errorf("whitespace: want a search with hits, got %d exact=%v", len(es), exact)
	}
}

func TestMathNames(t *testing.T) {
	c := mustLoad(t)
	for _, n := range []string{"sin", "pow", "fma", "log10"} {
		if es, exact := c.Lookup(n); !exact || es[0].Title != "Math" {
			t.Errorf("%s: want the Math section, got %d entries, exact %v", n, len(es), exact)
		}
	}
	for n, want := range map[string]string{"tojson": "Convert to/from JSON", "strftime": "Dates",
		"INDEX": "SQL-Style Operators", "@csv": "Format strings and escaping"} {
		if es, exact := c.Lookup(n); !exact || es[0].Title != want {
			t.Errorf("%s: want %q", n, want)
		}
	}
	if es, exact := c.Lookup("@foo"); exact {
		t.Errorf("@foo is the manual's placeholder, not a format: got %d entries", len(es))
	}
	if es, _ := c.Lookup("floor"); len(es) != 1 || es[0].Title != "`floor`" {
		t.Errorf("floor has its own entry; Math must not be added to it")
	}
}

func TestFunctions(t *testing.T) {
	fs := mustLoad(t).Functions()
	for _, want := range []string{"trim", "map", "sin", "keys_unsorted", "tojson", "strftime", "INDEX"} {
		if !slices.Contains(fs, want) {
			t.Errorf("Functions lacks %s", want)
		}
	}
	for _, not := range []string{"|", ".foo", "$ENV", "and", "reduce", "import", "@csv"} {
		if slices.Contains(fs, not) {
			t.Errorf("Functions has %q, which isn't a function", not)
		}
	}
	if !slices.IsSorted(fs) {
		t.Error("Functions should be sorted")
	}
}
