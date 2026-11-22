// Package table draws flat results as an aligned table (Q-056, V-017): a stream of objects, or
// one array of them, as rows, with their keys as columns.
//
// A Layout is made once per result from a sample of its rows, and then draws any window of it
// on demand, so a table of a million rows costs per frame what one of ten does (DESIGN §26).
// The TUI's table pane styles and scrolls a Layout; the REPL's :table prints one with Render.
package table

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/mattn/go-runewidth"
)

// Keyer gives an object's keys in the order to show them; shape.Order is one.
type Keyer interface {
	Keys(m map[string]any) []string
}

// Rows is where a table's rows come from: a slice of a result today, and later perhaps a
// paged or streaming source (DESIGN §26), which the drawing never needs to know about.
type Rows interface {
	Len() int
	Row(i int) map[string]any
}

type objects []map[string]any

func (o objects) Len() int                 { return len(o) }
func (o objects) Row(i int) map[string]any { return o[i] }

// FromValues reads a result as rows: a stream of objects, or one array of them. When they
// aren't, the error says which row isn't and what to try.
func FromValues(vs []any) (Rows, error) {
	rows := vs
	if len(vs) == 1 {
		if a, ok := vs[0].([]any); ok {
			rows = a
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no rows to show")
	}
	objs := make(objects, len(rows))
	for i, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("not a table: row %d is %s; a table is objects, or an array of them: try .[] | {a, b}", i+1, kind(r))
		}
		objs[i] = m
	}
	return objs, nil
}

// Kind is what a column holds, judged from the sample: it decides alignment now, and later
// what sort and the folds do with it (GRID §5, traps 15 and 16).
type Kind uint8

const (
	Empty   Kind = iota // nothing but null and missing
	Number              // JSON numbers
	NumText             // strings that all read as numbers: "3302"
	Text                // strings
	Mixed               // more than one of these, or nested values
)

// Column is one column of a layout.
type Column struct {
	Name  string
	Kind  Kind
	Width int // from the sample and the header, capped at Options.MaxWidth; Fit sets it

	Natural int // the widest value in the sample, or the header
	Typical int // the width nine values in ten fit in: one long outlier doesn't widen a column
}

// Right says whether the column's cells sit on the right: numbers do.
func (c Column) Right() bool { return c.Kind == Number }

// Options are how to lay out and draw.
type Options struct {
	Order    Keyer    // key order within a row; nil sorts
	Columns  []string // columns to put first, in this order: a projection's (DESIGN §26)
	Sample   int      // rows looked at for columns, kinds and widths; 0 means 1000
	MaxWidth int      // the widest a column gets; 0 means no cap

	// For Render:
	Width int // the width to fit by narrowing columns; 0 doesn't cut
	Max   int // how many rows to print; 0 prints all
}

// Layout is a result laid out as a table.
type Layout struct {
	Rows    Rows
	Columns []Column
}

const gap = "  "

// NewLayout lays rows out from a sample of them.
func NewLayout(rows Rows, o Options) *Layout {
	n := o.Sample
	if n <= 0 {
		n = 1000
	}
	n = min(n, rows.Len())

	var names []string
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			names = append(names, k)
		}
	}
	for _, k := range o.Columns {
		add(k)
	}
	sample := make([]map[string]any, n) // each row read once, whatever the source costs
	for i := range n {
		sample[i] = rows.Row(i)
	}
	for _, r := range sample {
		for _, k := range keys(r, o.Order) {
			add(k)
		}
	}

	l := &Layout{Rows: rows, Columns: make([]Column, len(names))}
	for j, name := range names {
		c := Column{Name: name, Width: runewidth.StringWidth(name)}
		var widths []int
		for _, r := range sample {
			v, ok := r[name]
			if !ok {
				continue
			}
			c.Kind = merge(c.Kind, kindOf(v))
			w := runewidth.StringWidth(cell(v))
			widths = append(widths, w)
			c.Width = max(c.Width, w)
		}
		c.Natural = c.Width
		if len(widths) > 0 {
			sort.Ints(widths)
			c.Typical = widths[(len(widths)*9+9)/10-1]
		}
		if o.MaxWidth > 0 {
			c.Width = min(c.Width, max(o.MaxWidth, 3))
		}
		l.Columns[j] = c
	}
	return l
}

// kindOf is one value's kind.
func kindOf(v any) Kind {
	switch v := v.(type) {
	case nil:
		return Empty
	case string:
		if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && v != "" {
			return NumText
		}
		return Text
	case bool, map[string]any, []any:
		return Mixed
	}
	return Number
}

// merge is a column's kind once it has also held a value of kind b.
func merge(a, b Kind) Kind {
	switch {
	case a == b || b == Empty:
		return a
	case a == Empty:
		return b
	case a == NumText && b == Text, a == Text && b == NumText:
		return Text
	}
	return Mixed
}

// Visible are the columns that fit in width, starting from column left: at least one, cut if
// it alone is too wide.
func (l *Layout) Visible(left, width int) []int {
	var cols []int
	used := 0
	for j := left; j < len(l.Columns); j++ {
		w := l.Columns[j].Width
		if len(cols) > 0 {
			w += len(gap)
		}
		if len(cols) > 0 && used+w > width {
			break
		}
		cols = append(cols, j)
		used += w
	}
	return cols
}

// Header is column j's name, fitted to its width as its cells are.
func (l *Layout) Header(j int) string {
	return fit(l.Columns[j].Name, l.Columns[j].Width, l.Columns[j].Right())
}

// Cell is row i's value in column j, fitted to the column: cut with … when long, on the right
// for numbers. A key the row lacks is blank; null says null.
func (l *Layout) Cell(i, j int) string {
	return fit(l.Full(i, j), l.Columns[j].Width, l.Columns[j].Right())
}

// Full is row i's value in column j, not cut: for the status line.
func (l *Layout) Full(i, j int) string {
	v, ok := l.Rows.Row(i)[l.Columns[j].Name]
	if !ok {
		return ""
	}
	return cell(v)
}

// Line joins fitted texts with the gap between columns, as Render does.
func Line(texts []string) string { return strings.Join(texts, gap) }

func fit(t string, width int, right bool) string {
	t = runewidth.Truncate(t, width, "…")
	pad := strings.Repeat(" ", max(0, width-runewidth.StringWidth(t)))
	if right {
		return pad + t
	}
	return t + pad
}

// Render prints vs as a table, or says why they aren't one: the REPL's :table.
func Render(vs []any, o Options) (string, error) {
	rows, err := FromValues(vs)
	if err != nil {
		return "", err
	}
	shown, more := rows.Len(), 0
	if o.Max > 0 && shown > o.Max {
		shown, more = o.Max, shown-o.Max
	}
	o.Sample = shown // every printed row: a printed table never cuts a cell it needn't
	l := NewLayout(rows, o)
	widths := make([]int, len(l.Columns))
	for j, c := range l.Columns {
		widths[j] = c.Width
	}
	narrow(widths, o.Width)
	for j := range l.Columns {
		l.Columns[j].Width = widths[j]
	}

	var b strings.Builder
	line := func(texts []string) {
		b.WriteString(strings.TrimRight(Line(texts), " ") + "\n")
	}
	texts := make([]string, len(l.Columns))
	for j := range l.Columns {
		texts[j] = l.Header(j)
	}
	line(texts)
	for j, c := range l.Columns {
		texts[j] = strings.Repeat("─", c.Width)
	}
	line(texts)
	for i := range shown {
		for j := range l.Columns {
			texts[j] = l.Cell(i, j)
		}
		line(texts)
	}
	if more > 0 {
		fmt.Fprintf(&b, "… %d more rows (%d in all)\n", more, rows.Len())
	}
	return b.String(), nil
}

// narrow takes the widest columns down, one at a time, until the table fits in width; a
// column keeps at least 3, so … and a little show.
func narrow(widths []int, width int) {
	if width <= 0 {
		return
	}
	total := len(gap) * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	for total > width {
		widest := 0
		for j, w := range widths {
			if w > widths[widest] {
				widest = j
			}
		}
		if widths[widest] <= 3 {
			return
		}
		widths[widest]--
		total--
	}
}

func keys(m map[string]any, o Keyer) []string {
	if o != nil {
		return o.Keys(m)
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// cell is a value as a table shows it: a string as it is (on one line), a nested value
// summarised, anything else as JSON.
func cell(v any) string {
	switch v := v.(type) {
	case string:
		return strings.NewReplacer("\r\n", "↵", "\n", "↵", "\t", " ").Replace(v)
	case map[string]any:
		if len(v) == 1 {
			return "{1 key}"
		}
		return fmt.Sprintf("{%d keys}", len(v))
	case []any:
		return fmt.Sprintf("[%d]", len(v))
	}
	b, err := gojq.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// kind names a value's type for a message: "a string", "an object".
func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return "a number"
}
