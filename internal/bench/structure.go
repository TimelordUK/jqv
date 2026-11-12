package bench

import (
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/shape"
)

// structure is the :structure command (Q-040): the shape of `.` (the document after the
// stages), or of expr's outputs, as an outline (shape.Outline). A leading number is a depth
// limit, `:structure 2 .fix`: a number alone has no structure worth asking for. With no
// stages and no expr the document's own tree is used, so keys come in the order written;
// otherwise gojq's values, their keys put back in the document's order where it has them.
func (s *Session) structure(arg string) string {
	o := shape.Options{}
	if first, rest, _ := strings.Cut(arg, " "); first != "" {
		if n, err := strconv.Atoi(first); err == nil && n >= 0 {
			o.Depth, arg = n, strings.TrimSpace(rest)
		}
	}
	if arg == "" && len(s.stages) == 0 && s.Doc != nil {
		return shape.Outline(shape.FromNode(s.Doc), o)
	}
	if arg == "" {
		arg = "."
	}
	vs, msg := s.values(arg)
	if msg != "" {
		return msg
	}
	if len(vs) == 0 {
		return "no outputs: nothing to show the structure of\n"
	}
	if s.keyOrder == nil && s.Doc != nil {
		s.keyOrder = shape.KeyOrder(s.Doc)
	}
	out := shape.Outline(shape.FromValues(vs, s.keyOrder), o)
	if len(vs) > 1 {
		out = strconv.Itoa(len(vs)) + " outputs, taken together:\n" + out
	}
	return out
}
