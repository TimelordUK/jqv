package tui

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
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
	styleKey       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleIndex     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7C7F93", Dark: "#7F849C"})
	styleString    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"})
	styleNumber    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FE640B", Dark: "#FAB387"})
	styleBool      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"})
	styleNull      = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.AdaptiveColor{Light: "#9CA0B0", Dark: "#6C7086"})
	styleSummary   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#6C7086"})
	styleMarker    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#9399B2"})
	styleHelpTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5B4BC4", Dark: "#B4A7F5"})
	styleHelpKey   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"})
	styleGuide     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#DCE0E8", Dark: "#313244"})
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
		b.WriteString(styleStatus.Render(pad(" ? esc q  close help", "", m.width)))
		return b.String()
	}
	for i := m.top; i < m.top+h; i++ {
		if i < len(rows) {
			b.WriteString(m.row(i, rows[i]))
		}
		b.WriteString("\n")
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
	left := " " + m.Cursor().Path()
	if m.status != "" {
		left = " " + m.status
	}
	if m.pending != "" {
		left += "  " + m.pending + "…"
	}
	right := fmt.Sprintf("? help  %d/%d ", m.cursor+1, m.tree.Len())
	return styleStatus.Render(pad(left, right, m.width))
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
func (m Model) row(i int, r tree.Row) string {
	if i == m.cursor {
		line := strings.Repeat("│ ", r.Depth) + m.tree.Label(r)
		line = ansi.Truncate(line, m.width, "…")
		return styleCursor.Render(line + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(line))))
	}

	var b strings.Builder
	for d := 0; d < r.Depth; d++ {
		b.WriteString(styleGuide.Render("│ "))
	}
	n := r.Node
	switch {
	case !n.IsContainer():
		b.WriteString("  ")
	case m.tree.Expanded(n):
		b.WriteString(styleMarker.Render("▾ "))
	default:
		b.WriteString(styleMarker.Render("▸ "))
	}
	if name := tree.Name(n); name != "" {
		if n.Parent.Kind == doc.Array {
			b.WriteString(styleIndex.Render(name))
		} else {
			b.WriteString(styleKey.Render(name))
		}
		if !n.IsContainer() {
			b.WriteString(styleSummary.Render(":"))
		}
		b.WriteString(" ")
	}
	b.WriteString(value(n))
	return ansi.Truncate(b.String(), m.width, "…")
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
