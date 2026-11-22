package table

import (
	"encoding/json"
	"strings"
	"testing"
)

func vals(t *testing.T, js ...string) []any {
	t.Helper()
	var vs []any
	for _, j := range js {
		d := json.NewDecoder(strings.NewReader(j))
		d.UseNumber()
		var v any
		if err := d.Decode(&v); err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	return vs
}

// insertion is the keys in the order the test writes them, standing in for shape.Order.
type insertion []string

func (o insertion) Keys(m map[string]any) []string {
	var ks []string
	for _, k := range o {
		if _, ok := m[k]; ok {
			ks = append(ks, k)
		}
	}
	return ks
}

// A stream of flat objects: columns in key order, numbers on the right, strings as they are.
func TestStream(t *testing.T) {
	vs := vals(t,
		`{"name":"Jodi Reyes","is":"E00001","age":30}`,
		`{"name":"Kimberly Perez","is":"E00002","age":7}`,
	)
	got, err := Render(vs, Options{Order: insertion{"name", "is", "age"}, Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"name            is      age\n" +
		"──────────────  ──────  ───\n" +
		"Jodi Reyes      E00001   30\n" +
		"Kimberly Perez  E00002    7\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// One array of objects is its elements; a key some rows lack is blank there, null says null,
// nested values are summarised, and a column first seen later comes after.
func TestArrayAndGaps(t *testing.T) {
	vs := vals(t, `[{"a":1,"b":null},{"a":2,"c":{"x":1,"y":2},"d":[1,2,3]},{"a":true}]`)
	got, err := Render(vs, Options{Order: insertion{"a", "b", "c", "d"}, Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"a     b     c         d\n" +
		"────  ────  ────────  ───\n" +
		"1     null\n" +
		"2           {2 keys}  [3]\n" +
		"true\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Too wide for the terminal: the widest columns give way, cut with …; every line fits.
func TestWidth(t *testing.T) {
	vs := vals(t, `{"id":"E1","bio":"`+strings.Repeat("words ", 30)+`","x":1}`)
	got, err := Render(vs, Options{Order: insertion{"id", "bio", "x"}, Width: 40})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if w := len([]rune(l)); w > 40 {
			t.Errorf("line %d wide: %q", w, l)
		}
	}
	if !strings.Contains(got, "…") || !strings.Contains(got, "E1") {
		t.Errorf("got\n%s", got)
	}
}

// Rows past Max are counted, not shown.
func TestMaxRows(t *testing.T) {
	var js []string
	for range 5 {
		js = append(js, `{"a":1}`)
	}
	got, _ := Render(vals(t, js...), Options{Width: 80, Max: 2})
	if !strings.HasSuffix(got, "… 3 more rows (5 in all)\n") || strings.Count(got, "\n") != 5 {
		t.Errorf("got\n%s", got)
	}
}

// Not a table: what is wrong, and what to try.
func TestNotFlat(t *testing.T) {
	for _, c := range []struct {
		js   []string
		want string
	}{
		{[]string{`{"a":1}`, `"x"`}, "row 2 is a string"},
		{[]string{`[1,2]`}, "row 1 is a number"},
		{[]string{`[]`}, "no rows"},
		{nil, "no rows"},
	} {
		_, err := Render(vals(t, c.js...), Options{Width: 80})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.js, err, c.want)
		}
	}
}
