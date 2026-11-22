// Package table draws flat results as an aligned table (Q-056): a stream of objects, or one
// array of them, as rows, with their keys as columns. The REPL's :table uses it now, and the
// TUI's output pane takes it up at DESIGN §25 step 2 (V-002).
package table

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/mattn/go-runewidth"
)

// Keyer gives an object's keys in the order to show them; shape.Order is one.
type Keyer interface {
	Keys(m map[string]any) []string
}

// Options are how to draw: the key order (nil sorts), the width to fit (0 doesn't cut), and
// how many rows to show (0 shows all).
type Options struct {
	Order Keyer
	Width int
	Max   int
}

const gap = "  "

// Render draws vs as a table, or says why they aren't one.
func Render(vs []any, o Options) (string, error) {
	rows := vs
	if len(vs) == 1 {
		if a, ok := vs[0].([]any); ok {
			rows = a
		}
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows to show")
	}
	objs := make([]map[string]any, len(rows))
	for i, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			return "", fmt.Errorf("not a table: row %d is %s; a table is objects, or an array of them: try .[] | {a, b}", i+1, kind(r))
		}
		objs[i] = m
	}
	more := 0
	if o.Max > 0 && len(objs) > o.Max {
		more = len(objs) - o.Max
		objs = objs[:o.Max]
	}

	var cols []string
	seen := map[string]bool{}
	for _, m := range objs {
		for _, k := range keys(m, o.Order) {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}

	cells := make([][]string, len(objs))
	widths := make([]int, len(cols))
	numeric := make([]bool, len(cols))
	for j, c := range cols {
		widths[j] = runewidth.StringWidth(c)
		numeric[j] = true
		found := false
		for _, m := range objs {
			v, ok := m[c]
			if !ok || v == nil {
				continue
			}
			found = true
			if kind(v) != "a number" {
				numeric[j] = false
			}
		}
		numeric[j] = numeric[j] && found
	}
	for i, m := range objs {
		cells[i] = make([]string, len(cols))
		for j, c := range cols {
			v, ok := m[c]
			if !ok {
				continue
			}
			cells[i][j] = cell(v)
			widths[j] = max(widths[j], runewidth.StringWidth(cells[i][j]))
		}
	}
	fit(widths, o.Width)

	var b strings.Builder
	line := func(texts []string) {
		var l strings.Builder
		for j, t := range texts {
			if j > 0 {
				l.WriteString(gap)
			}
			t = runewidth.Truncate(t, widths[j], "…")
			pad := strings.Repeat(" ", widths[j]-runewidth.StringWidth(t))
			if numeric[j] {
				l.WriteString(pad + t)
			} else {
				l.WriteString(t + pad)
			}
		}
		b.WriteString(strings.TrimRight(l.String(), " ") + "\n")
	}
	line(cols)
	rule := make([]string, len(cols))
	for j := range cols {
		rule[j] = strings.Repeat("─", widths[j])
	}
	line(rule)
	for _, r := range cells {
		line(r)
	}
	if more > 0 {
		fmt.Fprintf(&b, "… %d more rows (%d in all)\n", more, more+len(objs))
	}
	return b.String(), nil
}

// fit narrows the widest columns, one at a time, until the table fits in width; a column
// keeps at least 3, so … and a little show.
func fit(widths []int, width int) {
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
