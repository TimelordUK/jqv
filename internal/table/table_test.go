package table

import (
	"encoding/json"
	"fmt"
	"strconv"
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

func layout(t *testing.T, o Options, js ...string) *Layout {
	t.Helper()
	rows, err := FromValues(vals(t, js...))
	if err != nil {
		t.Fatal(err)
	}
	return NewLayout(rows, o)
}

// A column's kind comes from its values: numbers, numeric text (GRID trap 15), text, or a mix.
func TestKinds(t *testing.T) {
	l := layout(t, Options{Order: insertion{"n", "s", "t", "m", "e", "mix"}},
		`{"n":1,"s":"3302","t":"x","m":true,"e":null,"mix":"12"}`,
		`{"n":2.5,"s":" 99","t":"7","e":null,"mix":3}`,
		`{"n":null,"t":"y"}`,
	)
	want := map[string]Kind{"n": Number, "s": NumText, "t": Text, "m": Mixed, "e": Empty, "mix": Mixed}
	for _, c := range l.Columns {
		if c.Kind != want[c.Name] {
			t.Errorf("%s: kind %d, want %d", c.Name, c.Kind, want[c.Name])
		}
	}
	if !l.Columns[0].Right() || l.Columns[1].Right() {
		t.Error("numbers go right; numeric text stays as text for now")
	}
}

// Columns named in Options come first, in that order, then the rest as first seen.
func TestColumnsFirst(t *testing.T) {
	l := layout(t, Options{Order: insertion{"a", "b", "c"}, Columns: []string{"c", "a"}},
		`{"a":1,"b":2,"c":3}`)
	var got []string
	for _, c := range l.Columns {
		got = append(got, c.Name)
	}
	if strings.Join(got, " ") != "c a b" {
		t.Errorf("columns %v", got)
	}
}

// Widths come from the sample only, are capped, and cells are cut to them; Full is not.
func TestSampleAndCap(t *testing.T) {
	l := layout(t, Options{Sample: 2, MaxWidth: 6},
		`{"s":"ab"}`, `{"s":"abc"}`, `{"s":"a much longer value"}`)
	if w := l.Columns[0].Width; w != 3 {
		t.Errorf("width %d: the third row is outside the sample", w)
	}
	if got := l.Cell(2, 0); got != "a …" {
		t.Errorf("cut cell %q", got)
	}
	if got := l.Full(2, 0); got != "a much longer value" {
		t.Errorf("full %q", got)
	}
	l = layout(t, Options{MaxWidth: 6}, `{"s":"a much longer value"}`)
	if l.Columns[0].Width != 6 || l.Cell(0, 0) != "a muc…" {
		t.Errorf("capped: %d %q", l.Columns[0].Width, l.Cell(0, 0))
	}
}

// Visible is the columns that fit from left, gaps counted, and always at least one.
func TestVisible(t *testing.T) {
	l := layout(t, Options{Order: insertion{"a", "b", "c"}},
		`{"a":"aaaa","b":"bbbb","c":"cccc"}`) // widths 4, gaps 2
	for _, c := range []struct {
		left, width int
		want        string
	}{
		{0, 100, "[0 1 2]"},
		{0, 10, "[0 1]"},
		{0, 9, "[0]"},
		{1, 10, "[1 2]"},
		{2, 1, "[2]"},
		{0, 0, "[0]"},
	} {
		if got := fmt.Sprint(l.Visible(c.left, c.width)); got != c.want {
			t.Errorf("Visible(%d, %d) = %s, want %s", c.left, c.width, got, c.want)
		}
	}
	if got := Line([]string{l.Header(0), l.Cell(0, 1)}); got != "a     bbbb" {
		t.Errorf("line %q", got)
	}
}

// big is n rows of a TeamCity-like shape, without JSON in between.
type big int

func (b big) Len() int { return int(b) }
func (b big) Row(i int) map[string]any {
	return map[string]any{"Job": "Start Namespace - DEV" + strconv.Itoa(i%40), "Number": strconv.Itoa(3000 + i), "Took": i % 977}
}

// Drawing a window costs the same whatever the size (DESIGN §26): laying out 200k rows looks at
// the sample only, and a frame touches only the rows on screen.
func TestWindowCost(t *testing.T) {
	rows := &counting{Rows: big(200_000)}
	l := NewLayout(rows, Options{Order: insertion{"Job", "Number", "Took"}})
	if rows.n > 1000 {
		t.Errorf("layout read %d rows; the sample is 1000", rows.n)
	}
	rows.n = 0
	for i := 150_000; i < 150_030; i++ {
		for _, j := range l.Visible(0, 80) {
			_ = l.Cell(i, j)
		}
	}
	if rows.n > 30*3 {
		t.Errorf("a 30-row frame read %d rows", rows.n)
	}
	if got := l.Cell(150_000, 1); got != "153000" {
		t.Errorf("cell %q", got)
	}
}

type counting struct {
	Rows
	n int
}

func (c *counting) Row(i int) map[string]any { c.n++; return c.Rows.Row(i) }
