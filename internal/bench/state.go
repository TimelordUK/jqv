package bench

import (
	"fmt"
	"strings"
)

// State is the session's part of F12's dump (owner 2026-11-21): what is loaded, what is out
// in the editor, and the widths layout uses, to debug the program features.
func (s *Session) State() string {
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	cols := 0
	if s.Cols != nil {
		cols = s.Cols()
	}
	line("session: %d stages pushed; layout width %d (terminal %d columns, 0 = unknown)", len(s.stages), s.LayoutWidth(), cols)
	if s.prog == nil {
		line("program: none loaded")
	} else {
		p := s.prog
		line("program: %s, %d units, main from unit %d, changed %v, crlf %v", p.path, len(p.prog.Units), p.main, p.changed, p.crlf)
	}
	if s.edit != nil {
		line("editing: %s, runes %d-%d of the program, whole %v", s.edit.label, s.edit.from, s.edit.to, s.edit.whole)
	} else {
		line("editing: nothing out")
	}
	return b.String()
}
