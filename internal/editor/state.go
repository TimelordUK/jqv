package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// state is F12's dump (owner 2026-11-21): what the editor knows, to debug what it draws and
// where keys go, then the host's part (StateOf).
func (m *Model) state() string {
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	if m.sizes == 0 {
		line("terminal: size unknown (no size from Bubble Tea: the output isn't a console?)")
	} else {
		line("terminal: %d×%d (%d size messages)", m.width, m.height, m.sizes)
	}
	v := m.window()
	l, c := m.buf.lineCol()
	line("entry: %d lines, %d runes; cursor: line %d col %d (rune %d)", v.lines, len(m.buf.text), l+1, c+1, m.buf.cur)
	switch {
	case m.height <= 0:
		line("window: whole (height unknown)")
	case v.top == 0 && v.bottom == v.lines-1:
		line("window: whole (fits in %d rows less the hint)", m.height)
	default:
		line("window: lines %d-%d of %d, %d above, %d below, %d rows for text", v.top+1, v.bottom+1, v.lines, v.top, v.lines-1-v.bottom, m.budget())
	}
	walk := "no"
	if m.walking() {
		walk = "yes: up and down go through history"
	}
	line("prompt: %q (%d wide); history: %s %s; burst %v", m.Prompt, ansi.StringWidth(m.Prompt), walk, m.History.Status(), m.Burst)
	if m.StateOf != nil {
		b.WriteString(strings.TrimRight(m.StateOf(), "\n") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// budget is the rows a window gives the entry's text: what the preview leaves.
func (m *Model) budget() int { return max(3, m.height-spare-len(m.previewLines())) }

// page moves the cursor a window's height up (n < 0) or down, the window with it, so the
// cursor keeps its row, as a pager or editor does; it stops at the first or last line.
func (m *Model) page(dir int) {
	n := 20
	if m.height > 0 {
		n = m.budget()
	}
	moved := 0
	for moved < n {
		if (dir < 0 && !m.buf.Up()) || (dir > 0 && !m.buf.Down()) {
			break
		}
		moved++
	}
	m.top = max(0, m.top+dir*moved)
}

// lineCol is the cursor's line and column, from 0.
func (b *Buffer) lineCol() (line, col int) {
	start := b.lineStart(b.cur)
	for _, r := range b.text[:start] {
		if r == '\n' {
			line++
		}
	}
	return line, b.cur - start
}
