package program

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func examples(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../examples/jq/*/query.jq")
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	return files
}

// tiles: the units cover the text in order, nothing twice, nothing left out.
func tiles(t *testing.T, p *Program) {
	t.Helper()
	at, joined := 0, ""
	for i, u := range p.Units {
		if u.Pos != at || u.End < u.Pos || u.From < u.Pos || u.To < u.From || u.To > u.End {
			t.Fatalf("unit %d %s %q: %d..%d code %d..%d, after %d", i, u.Kind, u.Name, u.Pos, u.End, u.From, u.To, at)
		}
		at = u.End
		joined += p.Span(i)
	}
	if joined != p.Text {
		t.Fatalf("units joined don't give the text back")
	}
}

// TestExamples: every examples/jq program's units tile it, and its outline is as blessed.
func TestExamples(t *testing.T) {
	var b strings.Builder
	for _, f := range examples(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p := Parse(string(src))
		tiles(t, p)
		if !p.Tree.Whole() {
			t.Errorf("%s doesn't parse whole", f)
		}
		b.WriteString("== " + filepath.Base(filepath.Dir(f)) + "\n" + p.Outline())
	}
	golden := filepath.Join("testdata", "examples.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("outlines changed (go test ./internal/program -update, then review the diff):\n%s", got)
	}
}

func TestUnits(t *testing.T) {
	src := `# header

# items: a list
def items: [.];

def two: 2; # after
.a as $a
# the second
| $a | items
| def f: 1;
  f`
	p := Parse(src)
	tiles(t, p)
	var got []string
	for i, u := range p.Units {
		got = append(got, u.Kind.String()+" "+u.Name+" ["+p.Span(i)+"]")
	}
	want := []string{
		"header  [# header\n\n]",
		"def items [# items: a list\ndef items: [.];\n\n]",
		"def two [def two: 2; # after\n]",
		"stage as $a [.a as $a\n]",
		"stage  [# the second\n| $a ]",
		"stage  [| items\n]",
		"stage def f [| def f: 1;\n]",
		"stage  [  f]",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("units\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var eds []string
	for i := range p.Units {
		from, to := p.Editable(i)
		eds = append(eds, string([]rune(src)[from:to]))
	}
	wantEd := []string{"# header", "# items: a list\ndef items: [.];", "def two: 2; # after", ".a as $a",
		"# the second\n| $a", "| items", "| def f: 1;", "f"}
	if strings.Join(eds, "\n--\n") != strings.Join(wantEd, "\n--\n") {
		t.Errorf("editable\n%s\nwant\n%s", strings.Join(eds, "\n--\n"), strings.Join(wantEd, "\n--\n"))
	}
	if i := p.At(strings.Index(src, "items\n|")); p.Units[i].Name != "" || p.Code(i) != "| items" {
		t.Errorf("At on the second items is unit %d %q", i, p.Code(i))
	}
}

// TestReplace: replacing a unit changes its span and nothing else.
func TestReplace(t *testing.T) {
	src, err := os.ReadFile("../../examples/jq/fix-dictionary/query.jq")
	if err != nil {
		t.Fatal(err)
	}
	p := Parse(string(src))
	i := -1
	for j, u := range p.Units {
		if u.Name == "required" {
			i = j
		}
	}
	if i < 0 {
		t.Fatal("no def required")
	}
	old := p.Span(i)
	q := p.Replace(i, strings.Replace(old, `== "Y"`, `== "Y" or ._required == "y"`, 1))
	tiles(t, q)
	if len(q.Units) != len(p.Units) || !q.Tree.Whole() {
		t.Fatalf("after replacing, %d units (was %d), whole %v", len(q.Units), len(p.Units), q.Tree.Whole())
	}
	for j := range p.Units {
		if j != i && p.Span(j) != q.Span(j) {
			t.Errorf("unit %d changed: %q", j, q.Span(j))
		}
	}
	if !strings.Contains(q.Code(i), `._required == "y"`) {
		t.Errorf("unit %d is %q", i, q.Code(i))
	}
}

// FuzzParse: whatever the text, the units tile it.
func FuzzParse(f *testing.F) {
	f.Add("# h\n\ndef f: 1;\n.a as $x\n# c\n| $x | def g: 2; g")
	f.Add("def f: 1; # c\n")
	f.Add(".a | (")
	f.Fuzz(func(t *testing.T, s string) {
		if !strings.ContainsRune(s, 0) && strings.ToValidUTF8(s, "") == s {
			p := Parse(s)
			tiles(t, p)
			for i, u := range p.Units {
				if from, to := p.Editable(i); from < u.Pos || to < from || to > u.End {
					t.Fatalf("unit %d editable %d..%d outside its span %d..%d", i, from, to, u.Pos, u.End)
				}
			}
		}
	})
}
