package table

import (
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Floor is the narrowest Fit takes a column that would otherwise push the table past the
// pane: enough of a value to tell it apart; the rest is the status line's, or the cursor row's.
const Floor = 12

// Fit sets the columns' widths for a pane width cells wide (DESIGN §27). Every column starts
// at what it needs up to Floor; if that is too wide already, the rest is horizontal scroll.
// Spare width then goes, a column short by least first, up to the width its values typically
// need, and after that up to its widest. A column a few cells short of its widest counts its
// widest as typical, so ids mostly of three digits and a few of four show whole. Width 0
// leaves the widths as they are.
func (l *Layout) Fit(width int) {
	if width <= 0 || len(l.Columns) == 0 {
		return
	}
	ws := make([]int, len(l.Columns))
	typical := make([]int, len(l.Columns))
	natural := make([]int, len(l.Columns))
	spare := width - len(gap)*(len(ws)-1)
	for j, c := range l.Columns {
		natural[j] = max(c.Natural, 1)
		typical[j] = min(natural[j], max(c.Typical, runewidth.StringWidth(c.Name)))
		if natural[j]-typical[j] <= slack {
			typical[j] = natural[j]
		}
		ws[j] = min(typical[j], Floor)
		spare -= ws[j]
	}
	spare = fill(ws, typical, spare)
	fill(ws, natural, spare)
	for j := range l.Columns {
		l.Columns[j].Width = ws[j]
	}
}

// slack is how far short of its widest a column may be and still be given its widest as
// typical.
const slack = 4

// fill water-fills spare cells into ws towards target: the column short by least is filled
// first, each taking at most an even share of what is left. It gives back what is left.
func fill(ws, target []int, spare int) int {
	var short []int
	for j := range ws {
		if target[j] > ws[j] {
			short = append(short, j)
		}
	}
	sort.SliceStable(short, func(a, b int) bool {
		return target[short[a]]-ws[short[a]] < target[short[b]]-ws[short[b]]
	})
	for k, j := range short {
		if spare <= 0 {
			break
		}
		add := min(target[j]-ws[j], spare/(len(short)-k))
		ws[j] += add
		spare -= add
	}
	return spare
}

// Lines is row i's value in column j wrapped to its column's width, for the cursor row when it
// opens out (DESIGN §27): newlines in a string are kept, and words are broken only when one
// alone is wider than the column. At most max lines; when cut, the last ends in ….
func (l *Layout) Lines(i, j, most int) []string {
	c := l.Columns[j]
	v, ok := l.Rows.Row(i)[c.Name]
	if !ok {
		return []string{fit("", c.Width, false)}
	}
	text := cell(v)
	if s, ok := v.(string); ok {
		text = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\t", " ")
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, wrap(para, c.Width)...)
	}
	if most > 0 && len(out) > most {
		out = out[:most]
		out[most-1] = runewidth.Truncate(out[most-1]+" …", c.Width, "…")
		if !strings.HasSuffix(out[most-1], "…") {
			out[most-1] = runewidth.Truncate(out[most-1], c.Width-1, "") + "…"
		}
	}
	for k, s := range out {
		out[k] = fit(s, c.Width, c.Right())
	}
	return out
}

// wrap breaks s into lines of at most width cells, at spaces where it can.
func wrap(s string, width int) []string {
	width = max(1, width)
	var lines []string
	line := ""
	flush := func() { lines = append(lines, line); line = "" }
	for _, w := range strings.Fields(s) {
		for runewidth.StringWidth(w) > width { // a word too long for any line: break it
			if line != "" {
				flush()
			}
			head := runewidth.Truncate(w, width, "")
			lines = append(lines, head)
			w = w[len(head):]
		}
		switch {
		case line == "":
			line = w
		case runewidth.StringWidth(line)+1+runewidth.StringWidth(w) <= width:
			line += " " + w
		default:
			flush()
			line = w
		}
	}
	if line != "" || len(lines) == 0 {
		flush()
	}
	return lines
}
