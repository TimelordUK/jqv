package bench

import (
	"github.com/TimelordUK/jqv/internal/table"
)

// table is :table [expr] (Q-056): the last result (or expr's) as an aligned table, when it is
// a stream of objects or one array of them; columns in the order the keys were written
// (Q-010), cut to the terminal's width, rows stopping at Lines.
func (s *Session) table(expr string) string {
	vs, msg := s.values(expr)
	if msg != "" {
		return msg
	}
	width := s.Width
	if s.Cols != nil && s.Cols() > 0 {
		width = s.Cols() - 1
	}
	out, err := table.Render(vs, table.Options{Order: s.orderOf(expr), Width: width, Max: s.Lines})
	if err != nil {
		return err.Error() + "\n"
	}
	return out
}
