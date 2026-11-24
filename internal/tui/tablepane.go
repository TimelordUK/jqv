package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/TimelordUK/jqv/internal/pick"
	"github.com/TimelordUK/jqv/internal/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	styleBar     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4C4F69", Dark: "#CDD6F4"})
	styleColHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleColCur  = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"})
)

// openRows is the most lines the cursor row opens out to, to show long text whole (DESIGN §27).
const openRows = 6

// tablePane is the program and its result as a table (DESIGN §26, V-017): the bar with the
// jq, the run's count, and a window over the rows that scrolls both ways.
type tablePane struct {
	ws            *Workspace
	width, height int

	layout   *table.Layout // the result laid out; nil when it isn't a table (msg says why)
	msg      string
	shown    string // the program the layout is of
	row, top int    // cursor row, first row on screen
	col      int    // cursor column
	left     int    // first column on screen
	fitted   int    // the width the layout's columns were fitted to
	want     string // a column to put the cursor on when the next result is in: the one just picked
}

// refresh lays out the workspace's latest result, keeping the cursor on the same row number
// and column name, so picking one more field doesn't jump the table about.
func (p *tablePane) refresh(cols []string) {
	r := p.ws.Result()
	if r.Running && p.layout != nil {
		return // keep the last table on show until the new one is back
	}
	keep := ""
	if p.layout != nil && p.col < len(p.layout.Columns) {
		keep = p.layout.Columns[p.col].Name
	}
	p.layout, p.msg, p.shown = nil, "", r.Program
	switch {
	case r.Running:
		return
	case r.Err != nil:
		p.msg = r.Err.Error()
		return
	}
	rows, err := table.FromValues(r.Values)
	if err != nil {
		p.msg = err.Error()
		return
	}
	p.layout = table.NewLayout(rows, table.Options{Columns: cols})
	p.fitted = 0
	p.col = min(p.col, len(p.layout.Columns)-1)
	for j, c := range p.layout.Columns {
		if c.Name == keep {
			p.col = j
		}
	}
	if p.want != "" {
		for j, c := range p.layout.Columns {
			if c.Name == p.want {
				p.col = j
			}
		}
		p.want = ""
	}
	p.row = min(p.row, rows.Len()-1)
	p.scroll()
}

func (p tablePane) rows() int {
	if p.layout == nil {
		return 0
	}
	return p.layout.Rows.Len()
}

// bodyHeight is the rows of cells on screen: the pane less the bar, the count, the header and
// the status line.
func (p tablePane) bodyHeight() int { return max(1, p.height-3-len(p.bar())) }

// bar is the program, read only for now (the editor comes later, Q-002), a stage to a line
// or more (DESIGN §28.2): picks laid out by pick.Pretty to the pane's width, other stages cut
// into lines as they are, each after the first starting `| `. With more than one stage, or
// the cursor past the first, a gutter marks the cursor's stage, and the stages above it,
// locked, are dim. It takes up to a third of the pane; past that the last line ends in ….
func (p tablePane) bar() []string {
	st, at := p.ws.Stages(), p.ws.At()
	gutter := len(st) > 1 || at > 0
	w := max(1, p.width-1)
	if gutter {
		w = max(1, p.width-3)
	}
	var lines []string
	var dim []bool
	for i, s := range st {
		var ls []string
		if s.Kind == pick.Picks {
			ls = pick.Pretty(s.Form, w)
		} else {
			text := s.Text
			if i > 0 {
				text = "| " + text
			}
			ls = strings.Split(ansi.Hardwrap(text, w, true), "\n")
		}
		if i > 0 && s.Kind == pick.Picks {
			ls[0] = "| " + ls[0]
		}
		for j, l := range ls {
			switch {
			case !gutter:
				l = " " + l
			case i == at && j == 0:
				l = "▸ " + l
			default:
				l = "  " + l
			}
			lines = append(lines, l)
			dim = append(dim, i < at)
		}
	}
	if gutter && at == len(st) {
		lines = append(lines, "▸ ·  the end: = on a value adds a filter · to or with it, K onto it and | a value")
		dim = append(dim, false)
	}
	if most := max(1, p.height/3); len(lines) > most {
		lines = lines[:most]
		lines[most-1] += " …"
	}
	for i, l := range lines {
		l = ansi.Truncate(l, p.width, "…")
		if dim[i] {
			lines[i] = styleSummary.Render(l)
		} else {
			lines[i] = styleBar.Render(l)
		}
	}
	return lines
}

func (p *tablePane) key(k string) tea.Cmd {
	n := p.rows()
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "j", "down":
		p.row++
	case "k", "up":
		p.row--
	case "ctrl+d":
		p.row += max(1, p.bodyHeight()/2)
	case "ctrl+u":
		p.row -= max(1, p.bodyHeight()/2)
	case "ctrl+f", "pgdown":
		p.row += p.bodyHeight()
	case "ctrl+b", "pgup":
		p.row -= p.bodyHeight()
	case "G", "end":
		p.row = n - 1
	case "home":
		p.row = 0
	case "l", "right":
		p.col++
	case "h", "left":
		p.col--
	case "0":
		p.col = 0
	case "$":
		p.col = len(p.columns()) - 1
	}
	p.scroll()
	return nil
}

func (p tablePane) columns() []table.Column {
	if p.layout == nil {
		return nil
	}
	return p.layout.Columns
}

// scroll keeps the cursor in range and on screen, both ways.
func (p *tablePane) scroll() {
	p.row = max(0, min(p.row, p.rows()-1))
	p.col = max(0, min(p.col, len(p.columns())-1))
	p.left = max(0, min(p.left, p.col))
	if p.layout != nil {
		if p.fitted != p.width {
			p.layout.Fit(p.width - 1)
			p.fitted = p.width
		}
		for p.left < p.col && !contains(p.layout.Visible(p.left, p.width-1), p.col) {
			p.left++
		}
	}
	// The cursor row may be open over several lines; all of them stay on screen.
	h := p.bodyHeight() - (p.cursorHeight() - 1)
	if p.row < p.top {
		p.top = p.row
	}
	if p.row >= p.top+h {
		p.top = p.row - h + 1
	}
	p.top = max(0, min(p.top, p.rows()-h))
}

func contains(xs []int, x int) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// view draws the pane in exactly height lines of width cells.
func (p tablePane) view() string {
	var lines []string
	lines = append(lines, p.bar()...)
	lines = append(lines, styleGuide.Render(rule(p.count(), p.width)))
	switch {
	case p.layout == nil && p.msg != "":
		for _, l := range strings.Split(p.msg, "\n") {
			lines = append(lines, styleError.Render(ansi.Truncate(" "+l, p.width, "…")))
		}
	case p.layout != nil:
		vis := p.layout.Visible(p.left, p.width-1)
		texts := make([]string, len(vis))
		for i, j := range vis {
			st := styleColHead
			if j == p.col {
				st = styleColCur
			}
			texts[i] = st.Render(p.layout.Header(j))
		}
		lines = append(lines, " "+table.Line(texts))
		used := 0
		for i := p.top; i < p.rows() && used < p.bodyHeight(); i++ {
			if i == p.row {
				for _, l := range p.cursorLines(vis) {
					l = ansi.Truncate(l, p.width, "…")
					lines = append(lines, styleCursor.Render(l+strings.Repeat(" ", max(0, p.width-ansi.StringWidth(l)))))
					used++
				}
				continue
			}
			for x, j := range vis {
				texts[x] = p.layout.Cell(i, j)
			}
			lines = append(lines, " "+table.Line(texts))
			used++
		}
	}
	for len(lines) < p.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:p.height-1], p.statusBar())
	return strings.Join(lines, "\n")
}

// count is the line under the bar: rows, columns and time, or that the run is out.
func (p tablePane) count() string {
	r := p.ws.Result()
	switch {
	case r.Running:
		return " running… "
	case p.layout == nil:
		return ""
	}
	s := fmt.Sprintf(" %s · %s · %s ", plural(p.rows(), "row"), plural(len(p.layout.Columns), "column"), took(r.Took))
	if r.More {
		s = " PREVIEW: stopped at" + s
	}
	return s
}

func took(d time.Duration) string {
	if d < time.Millisecond {
		return "<1 ms"
	}
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// rule is a horizontal line with text set into it near the start: `─ 120 rows ───`.
func rule(text string, width int) string {
	if text == "" {
		return strings.Repeat("─", max(0, width))
	}
	text = ansi.Truncate(text, max(0, width-1), "…")
	return "─" + text + strings.Repeat("─", max(0, width-1-ansi.StringWidth(text)))
}

func (p tablePane) statusBar() string {
	left, right := "", ""
	if p.layout != nil && p.rows() > 0 {
		c := p.layout.Columns[p.col]
		left = " " + c.Name + ": " + p.layout.Full(p.row, p.col)
		right = fmt.Sprintf("table %d/%d ", p.row+1, p.rows())
	}
	return styleStatus.Render(pad(left, right, p.width))
}

// cursorLines is the cursor row opened out: each visible cell wrapped to its column, as many
// lines as the longest needs up to openRows, so long text is read in place (DESIGN §27).
func (p tablePane) cursorLines(vis []int) []string {
	cells := make([][]string, len(vis))
	n := 1
	for x, j := range vis {
		cells[x] = p.layout.Lines(p.row, j, openRows)
		n = max(n, len(cells[x]))
	}
	n = min(n, max(1, p.bodyHeight()))
	out := make([]string, n)
	texts := make([]string, len(vis))
	for k := range n {
		for x, j := range vis {
			if k < len(cells[x]) {
				texts[x] = cells[x][k]
			} else {
				texts[x] = strings.Repeat(" ", p.layout.Columns[j].Width)
			}
		}
		out[k] = " " + table.Line(texts)
	}
	return out
}

// cursorHeight is how many lines the cursor row takes on screen.
func (p tablePane) cursorHeight() int {
	if p.layout == nil || p.rows() == 0 {
		return 1
	}
	return len(p.cursorLines(p.layout.Visible(p.left, p.width-1)))
}
