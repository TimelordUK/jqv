package scope

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/query"
	"github.com/TimelordUK/jqv/internal/shape"
)

// shapes are Shapes on a document in the repository's testdata, run as the REPL does.
func shapes(t *testing.T, name string) (*Shapes, *int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	n, err := doc.Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	root := n.Value()
	runs := 0
	return &Shapes{
		Input: shape.FromNode(n),
		Order: shape.KeyOrder(n),
		Run: func(p string) ([]any, bool, error) {
			runs++
			r := query.Run(p, root, map[string]any{"$root": root}, query.Options{Timeout: time.Second, Max: 1000})
			return r.Values, r.More || r.Late, r.Err
		},
	}, &runs
}

func describeFound(f Found) string {
	var b strings.Builder
	switch {
	case f.Missing != "":
		b.WriteString("missing " + f.Missing)
	case f.Shape == nil:
		b.WriteString("nothing")
	default:
		b.WriteString(shape.Brief(f.Shape, 100))
	}
	if f.Ran {
		b.WriteString("  [ran]")
	}
	if f.Cut {
		b.WriteString("  [cut]")
	}
	if f.Absent > 0 {
		fmt.Fprintf(&b, "  [absent %d]", f.Absent)
	}
	if f.Err != nil {
		b.WriteString("  [error: " + f.Err.Error() + "]")
	}
	return b.String()
}

func TestShapesOf(t *testing.T) {
	s, runs := shapes(t, "employees_5-level_10-KB_formatted.json")
	cases := []struct{ program, want string }{
		{`.`, `array  len 6 of object ×6 {employee}`},
		{`.[].employee`, `object ×6 {id name position department projects}`},
		{`.[].employee | .projects`, `array ×6  len 1 of object ×6 {projectId projectName startDate tasks}`},
		{`.[].employee | .projects[0]`, `object ×6 {projectId projectName startDate tasks}  [ran]`},
		{`.[] | select(.employee.name == "Jodi Reyes") | .employee`, `object {id name position department projects}  [ran]`},
		{`.[].employee | .projects[0].id`, `null ×6  [ran]`},
		{`.[].employee | .proj`, `missing .proj`},
		{`.[].employee.projects[].tasks[].details | .completionDate`, `string ×8  all distinct  e.g. "2025-02-12"  [absent 4]`},
		{`.[].employee.projects[].tasks[] | .status`, `string ×12  "Completed" ×8 "In Progress" ×2 "Pending" ×2`},
		{`.[].employee.name | .x`, `nothing  [ran]  [error: expected an object but got: string ("Jodi Reyes")]`},
		{`range(1e9)`, `number ×1000  all distinct  0..999  e.g. 0  [ran]  [cut]`},
	}
	for _, c := range cases {
		if got := describeFound(s.Of(c.program)); got != c.want {
			t.Errorf("%s\n  got  %s\n  want %s", c.program, got, c.want)
		}
	}
	// a run is kept: the same prefix with another path after it runs nothing more
	n := *runs
	s.Of(`.[] | select(.employee.name == "Jodi Reyes") | .employee.projects`)
	s.Of(`.[] | select(.employee.name == "Jodi Reyes") | .employee`)
	if *runs != n {
		t.Errorf("%d more runs, want none", *runs-n)
	}
}

// Brief stays one line on a big document: FIX4.4's fields by number have 912 keys.
func TestBriefBig(t *testing.T) {
	s, _ := shapes(t, "FIX4.4.json")
	f := s.Of(`.fix.fields.field | INDEX(._number)`)
	got := shape.Brief(f.Shape, 100)
	if n := len([]rune(got)); n > 110 || !strings.HasSuffix(got, " more}") {
		t.Errorf("%d runes: %s", n, got)
	}
}
