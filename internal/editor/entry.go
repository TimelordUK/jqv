package editor

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// EntryKind is what the host says the entry is, which changes a few keys (Q-049, DESIGN §23).
type EntryKind int

const (
	// EntryLine is a line typed at the prompt: enter runs it, up and down reach history.
	EntryLine EntryKind = iota
	// EntryUnit is a part of a program out for editing (:e name): up and down stop at its first and
	// last lines instead of walking history.
	EntryUnit
	// EntryProgram is a whole program out for editing (:e alone): as Unit, and enter breaks a line,
	// esc puts it back, and it isn't kept in history.
	EntryProgram
)

// kind is the entry's kind, EntryLine when the host doesn't say.
func (m *Model) kind() EntryKind {
	if m.Entry == nil {
		return EntryLine
	}
	return m.Entry()
}

// oneRow is s on one row of the terminal: a line break shown as ⏎, cut to the width with …,
// since the window leaves one row for what shows under the entry.
func (m *Model) oneRow(s string) string {
	s = strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "⏎")
	if m.width > 0 && ansi.StringWidth(s) > m.width-1 {
		s = ansi.Truncate(s, m.width-1, "…")
	}
	return s
}
