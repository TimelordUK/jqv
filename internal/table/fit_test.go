package table

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func fitRows(n int, long string) Rows {
	var rs objects
	for i := range n {
		rs = append(rs, map[string]any{"id": float64(i), "share": "1", "motivation": long})
	}
	return rs
}

// DESIGN §27: short columns take what they need, the long one gets the rest of the pane.
func TestFitGivesSpareToLongColumns(t *testing.T) {
	l := NewLayout(fitRows(50, strings.Repeat("word ", 40)), Options{Columns: []string{"id", "share", "motivation"}})
	l.Fit(80)
	id, share, mot := l.Columns[0].Width, l.Columns[1].Width, l.Columns[2].Width
	if id != 2 || share != 5 {
		t.Errorf("id %d share %d, want their natural widths 2 and 5", id, share)
	}
	if total := id + share + mot + 2*len(gap); total != 80 {
		t.Errorf("table %d wide, want the pane's 80 (motivation %d)", total, mot)
	}
}

// One long outlier doesn't widen its column when space is short.
func TestFitTypicalNotOutlier(t *testing.T) {
	var rs objects
	for i := range 20 {
		v := "short"
		if i == 7 {
			v = strings.Repeat("x", 200)
		}
		rs = append(rs, map[string]any{"a": v, "b": strings.Repeat("y", 100)})
	}
	l := NewLayout(rs, Options{Columns: []string{"a", "b"}})
	if l.Columns[0].Typical != 5 || l.Columns[0].Natural != 200 {
		t.Fatalf("a: typical %d natural %d", l.Columns[0].Typical, l.Columns[0].Natural)
	}
	l.Fit(40)
	if l.Columns[0].Width > Floor || l.Columns[1].Width < Floor {
		t.Errorf("widths %d %d", l.Columns[0].Width, l.Columns[1].Width)
	}
}

// Too wide even at their typical widths: the widest come down to Floor, no further.
func TestFitFloor(t *testing.T) {
	row := map[string]any{}
	var cols []string
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		row[k] = strings.Repeat(k, 30)
		cols = append(cols, k)
	}
	l := NewLayout(objects{row}, Options{Columns: cols})
	l.Fit(40)
	for _, c := range l.Columns {
		if c.Width != Floor {
			t.Errorf("%s is %d, want %d", c.Name, c.Width, Floor)
		}
	}
}

// The cursor row's lines: words kept whole, newlines kept, cut at most with ….
func TestLines(t *testing.T) {
	l := NewLayout(objects{{"m": "for the discovery of non-linear effects\nin synthesis", "n": 3.0}}, Options{Columns: []string{"m", "n"}})
	l.Columns[0].Width = 15
	got := l.Lines(0, 0, 0)
	want := []string{"for the        ", "discovery of   ", "non-linear     ", "effects        ", "in synthesis   "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
	cut := l.Lines(0, 0, 2)
	if len(cut) != 2 || !strings.Contains(cut[1], "…") || runewidth.StringWidth(cut[1]) != 15 {
		t.Errorf("cut %q", cut)
	}
	if n := l.Lines(0, 1, 6); len(n) != 1 || strings.TrimSpace(n[0]) != "3" {
		t.Errorf("number %q", n)
	}
	if w := wrap(strings.Repeat("z", 23), 10); len(w) != 3 || w[2] != "zzz" {
		t.Errorf("long word %q", w)
	}
}

// From daily use (Nobel): ids are mostly three digits, a few four; with room to spare the
// column shows them whole rather than 10….
func TestFitShortFirst(t *testing.T) {
	var rs objects
	for i := range 100 {
		id := "106"
		if i%15 == 0 {
			id = "1065"
		}
		rs = append(rs, map[string]any{"id": id, "m": strings.Repeat("w ", 100)})
	}
	l := NewLayout(rs, Options{Columns: []string{"id", "m"}})
	l.Fit(60)
	if l.Columns[0].Width != 4 || l.Columns[0].Width+l.Columns[1].Width+len(gap) != 60 {
		t.Errorf("id %d, m %d", l.Columns[0].Width, l.Columns[1].Width)
	}
}
