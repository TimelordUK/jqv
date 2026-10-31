package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/find"
	"github.com/TimelordUK/jqv/internal/tree"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette. Adaptive pairs read on both light and dark terminals; themes are a wishlist item (T-008).
var (
	styleHeader = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E1E2E"}).
			Background(lipgloss.AdaptiveColor{Light: "#5B4BC4", Dark: "#B4A7F5"})
	styleStatus = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CDD6F4"}).
			Background(lipgloss.AdaptiveColor{Light: "#E4E4EC", Dark: "#313244"})
	styleCursor = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#D0D7F5", Dark: "#45475A"})
	styleKey          = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleIndex        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7C7F93", Dark: "#7F849C"})
	styleString       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"})
	styleNumber       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FE640B", Dark: "#FAB387"})
	styleBool         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"})
	styleNull         = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.AdaptiveColor{Light: "#9CA0B0", Dark: "#6C7086"})
	styleSummary      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#6C7086"})
	styleMarker       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#9399B2"})
	styleHelpTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5B4BC4", Dark: "#B4A7F5"})
	styleHelpKey      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleGutter       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9CA0B0", Dark: "#585B70"})
	styleGutterCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5B4BC4", Dark: "#B4A7F5"})
	styleGuide        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#DCE0E8", Dark: "#313244"})
	styleMatch        = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.Color("#1E1E2E")).
				Background(lipgloss.Color("#F9E2AF"))
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	rows := m.tree.Rows()
	h := m.bodyHeight()
	if m.help {
		lines := helpLines()
		for i := 0; i < h; i++ {
			if i < len(lines) {
				b.WriteString(ansi.Truncate(lines[i], m.width, "…"))
			}
			b.WriteString("\n")
		}
		b.WriteString(styleStatus.Render(pad(" F1 esc q  close help", "", m.width)))
		return b.String()
	}
	for i := m.top; i < m.top+h; i++ {
		if i < len(rows) {
			g := m.gutter(i)
			b.WriteString(g)
			b.WriteString(m.row(i, rows[i], m.width-ansi.StringWidth(g)))
		}
		b.WriteString("\n")
	}
	if m.listRows() > 0 {
		b.WriteString(m.hitList())
	}
	b.WriteString(m.statusBar())
	return b.String()
}

func (m Model) header() string {
	left := " jqv  " + m.Title
	right := fmt.Sprintf("%s ", tree.Summary(m.tree.Root))
	if !m.tree.Root.IsContainer() {
		right = m.tree.Root.Kind.String() + " "
	}
	return styleHeader.Render(pad(left, right, m.width))
}

func (m Model) statusBar() string {
	if m.prompt != "" {
		return styleStatus.Render(pad(" "+m.prompt+m.input+"█", m.promptCount(), m.width))
	}
	left := " " + m.Cursor().Path()
	if m.status != "" {
		left = " " + m.status
	}
	if m.pending != "" {
		left += "  " + m.pending + "…"
	}
	right := fmt.Sprintf("F1 help  %d/%d ", m.cursor+1, m.tree.Len())
	if m.lock {
		right = "locked  " + right
	}
	if !m.query.Empty() {
		if i := find.Index(m.matches, m.Cursor()); i >= 0 {
			right = fmt.Sprintf("match %d/%d  ", i+1, len(m.matches)) + right
		} else {
			right = fmt.Sprintf("%d matches  ", len(m.matches)) + right
		}
	}
	return styleStatus.Render(pad(left, right, m.width))
}

// promptCount is the right of the status bar while a search is being typed.
func (m Model) promptCount() string {
	switch {
	case m.prompt == ":" || m.input == "":
		return ""
	case m.finding() && len(m.matches) > 0:
		return fmt.Sprintf("%d/%d ", m.pick+1, len(m.matches))
	}
	return fmt.Sprintf("%d matches ", len(m.matches))
}

// hitList draws the finder's list under the tree, each hit as the path of its parent and then
// the row the tree would show, so a column of `id:` lines still says which record each is in.
func (m Model) hitList() string {
	var b strings.Builder
	b.WriteString(styleGuide.Render(strings.Repeat("─", max(0, m.width))))
	b.WriteString("\n")

	end := min(m.pickTop+m.listRows(), len(m.matches))
	col := 0
	for _, n := range m.matches[min(m.pickTop, end):end] {
		col = max(col, ansi.StringWidth(parentPath(n)))
	}
	col = min(col, m.width/2)
	for i := m.pickTop; i < m.pickTop+m.listRows(); i++ {
		if i < end {
			b.WriteString(m.hit(i, col))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func parentPath(n *doc.Node) string {
	if n.Parent == nil {
		return ""
	}
	return n.Parent.Path()
}

func (m Model) hit(i, col int) string {
	n := m.matches[i]
	path := fitLeft(parentPath(n), col)
	if i == m.pick {
		line := ansi.Truncate(path+"  "+m.tree.Label(tree.Row{Node: n}), m.width, "…")
		return styleCursor.Render(line + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(line))))
	}
	return ansi.Truncate(styleSummary.Render(path)+"  "+m.label(n), m.width, "…")
}

// fitLeft fits s to exactly width cells. A long path loses its start: the end says more.
func fitLeft(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return "…" + ansi.TruncateLeft(s, w-width+1, "")
	}
	return s + strings.Repeat(" ", width-w)
}

// helpLines renders the keymap table: a heading per group, then `keys   what`.
func helpLines() []string {
	var lines []string
	for _, g := range keymap {
		lines = append(lines, "")
		if g.title != "" {
			lines = append(lines, " "+styleHelpTitle.Render(g.title))
		}
		for _, b := range g.bindings {
			keys := strings.ReplaceAll(b.keys, " ", " · ")
			if b.keys == "" { // a note under the group
				lines = append(lines, "   "+styleSummary.Render(b.what))
				continue
			}
			lines = append(lines, "   "+styleHelpKey.Render(fmt.Sprintf("%-16s", keys))+b.what)
		}
	}
	return lines
}

// pad lays left and right text across width, cutting left if they do not fit.
func pad(left, right string, width int) string {
	room := width - ansi.StringWidth(right)
	left = ansi.Truncate(left, max(0, room), "…")
	gap := max(0, room-ansi.StringWidth(left))
	return left + strings.Repeat(" ", gap) + right
}

// row draws one tree line: indent guides, marker, name, value.
// gutter is the row number column (:set nu / rnu), or "" when off. With both on, the cursor's
// row shows its own number and the others their distance, as vim does.
func (m Model) gutter(i int) string {
	if !m.number && !m.relnum {
		return ""
	}
	w := len(strconv.Itoa(m.tree.Len()))
	n := i + 1
	if m.relnum && i != m.cursor {
		n = i - m.cursor
		if n < 0 {
			n = -n
		}
	} else if m.relnum && !m.number {
		n = 0
	}
	style := styleGutter
	if i == m.cursor {
		style = styleGutterCursor
	}
	return style.Render(fmt.Sprintf("%*d ", w, n))
}

func (m Model) row(i int, r tree.Row, width int) string {
	if i == m.cursor {
		line := strings.Repeat("│ ", r.Depth) + m.tree.Label(r)
		line = ansi.Truncate(line, width, "…")
		return styleCursor.Render(line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line))))
	}

	var b strings.Builder
	for d := 0; d < r.Depth; d++ {
		b.WriteString(styleGuide.Render("│ "))
	}
	b.WriteString(m.label(r.Node))
	return ansi.Truncate(b.String(), width, "…")
}

// label is the coloured text of a node's row, without indentation: marker, name, value.
func (m Model) label(n *doc.Node) string {
	var b strings.Builder
	switch {
	case !n.IsContainer():
		b.WriteString("  ")
	case m.tree.Expanded(n):
		b.WriteString(styleMarker.Render("▾ "))
	default:
		b.WriteString(styleMarker.Render("▸ "))
	}
	if name := tree.Name(n); name != "" {
		switch {
		case m.query.MatchKey(n):
			b.WriteString(styleMatch.Render(name))
		case n.Parent.Kind == doc.Array:
			b.WriteString(styleIndex.Render(name))
		default:
			b.WriteString(styleKey.Render(name))
		}
		if !n.IsContainer() {
			b.WriteString(styleSummary.Render(":"))
		}
		b.WriteString(" ")
	}
	if m.query.MatchValue(n) {
		b.WriteString(styleMatch.Render(tree.Scalar(n)))
	} else {
		b.WriteString(value(n))
	}
	return b.String()
}

func value(n *doc.Node) string {
	switch n.Kind {
	case doc.Object, doc.Array:
		return styleSummary.Render(tree.Summary(n))
	case doc.String:
		return styleString.Render(tree.Scalar(n))
	case doc.Number:
		return styleNumber.Render(n.Raw)
	case doc.Bool:
		return styleBool.Render(n.Raw)
	default:
		return styleNull.Render(n.Raw)
	}
}
