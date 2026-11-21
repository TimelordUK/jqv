package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The window (Q-049, DESIGN §23): the REPL draws inline, and Bubble Tea's inline view can't be
// taller than the terminal, so an entry that is (a whole program in buffer mode) shows the
// lines around the cursor, with how many are hidden above and below. It moves only when the
// cursor goes past an edge, as an editor's does.

// spare is the rows a windowed view keeps for other things: the ↑ and ↓ markers, the hint
// line under the entry, and one so the view never fills the terminal.
const spare = 4

// view is the part of the entry shown: lines top to bottom (inclusive), and the runes they
// span, from to to.
type view struct {
	top, bottom int
	from, to    int
	lines       int // the entry's lines
}

// window is the view to draw: the whole entry when it fits the terminal (or the terminal's
// height isn't known), else a window holding the cursor's line, kept from m.top where it can.
func (m *Model) window() view {
	starts := []int{0}
	for i, r := range m.buf.text {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	n := len(starts)
	end := func(i int) int { // where line i ends, before its break
		if i+1 < n {
			return starts[i+1] - 1
		}
		return len(m.buf.text)
	}
	whole := view{0, n - 1, 0, len(m.buf.text), n}
	if m.height <= 0 {
		return whole
	}
	pw := ansi.StringWidth(m.Prompt)
	rows := func(i int) int {
		w := pw + ansi.StringWidth(string(m.buf.text[starts[i]:end(i)])) + 1 // + 1: a cursor at its end
		if m.width <= 0 {
			return 1
		}
		return max(1, (w+m.width-1)/m.width)
	}
	total := 0
	for i := range n {
		total += rows(i)
	}
	if total <= m.height-2 { // the entry and the hint line
		return whole
	}
	budget := m.budget()
	c := 0
	for c+1 < n && starts[c+1] <= m.buf.cur {
		c++
	}
	top := min(m.top, c)
	used := 0
	for i := top; i <= c; i++ {
		used += rows(i)
	}
	for top < c && used > budget { // the cursor went below the window: scroll down
		used -= rows(top)
		top++
	}
	bottom := c
	for bottom+1 < n && used+rows(bottom+1) <= budget {
		bottom++
		used += rows(bottom)
	}
	for top > 0 && used+rows(top-1) <= budget { // room left at the end (lines deleted): show more above
		top--
		used += rows(top)
	}
	return view{top, bottom, starts[top], end(bottom), n}
}

// scroll keeps the window where it now is, for the next key.
func (m *Model) scroll() { m.top = m.window().top }

// entry draws the entry's lines in view v, painted, with the cursor at cur (none if < 0): the
// prompt before the first line shown, or before a ↑ marker when lines are hidden above, and a
// ↓ marker after when lines are hidden below.
func (m *Model) entry(v view, spans []Span, cur int) string {
	if cur >= 0 {
		cur -= v.from
	}
	body := Render(m.buf.text[v.from:v.to], within(spans, v.from, v.to), cur)
	s := m.Prompt + m.continued(body)
	if v.top > 0 {
		s = m.Prompt + m.dim(fmt.Sprintf("↑ %d more", v.top)) + m.continued("\n"+body)
	}
	if below := v.lines - 1 - v.bottom; below > 0 {
		s += "\n" + strings.Repeat(" ", ansi.StringWidth(m.Prompt)) + m.dim(fmt.Sprintf("↓ %d more", below))
	}
	return s
}

// within is spans cut to from..to and counted from from.
func within(spans []Span, from, to int) []Span {
	if from == 0 {
		return spans
	}
	var out []Span
	for _, sp := range spans {
		f, t := max(sp.From, from), min(sp.To, to)
		if f < t {
			out = append(out, Span{f - from, t - from, sp.Style})
		}
	}
	return out
}
