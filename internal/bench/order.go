package bench

import (
	"bytes"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/shape"
	"github.com/itchyny/gojq"
)

// order is how expr's objects print their keys (Q-010): gojq's maps forget the order, so an
// object the program builds takes the order its literal is written in, and one passed
// through the document's; nil, sorted as gojq gives them, while :sortkeys is on.
func (s *Session) order(expr string) *shape.Order {
	if s.sortKeys {
		return nil
	}
	if s.keyOrder == nil && s.Doc != nil {
		s.keyOrder = shape.KeyOrder(s.Doc)
	}
	return s.keyOrder.With(objectKeys(s.Program(expr)))
}

// orderOf is order for a command's argument: expr's, or the last line's when it is empty.
func (s *Session) orderOf(expr string) *shape.Order {
	if expr == "" {
		expr = s.last
	}
	return s.order(expr)
}

// objectKeys are the keys of each object literal in prog, as written: {name, is: .id} gives
// [name is]. A key worked out at run time, {(.k): 1}, can't be known and is left out.
func objectKeys(prog string) [][]string {
	var sets [][]string
	jqsyntax.Walk(jqsyntax.Parse(prog), func(n *jqsyntax.Node) bool {
		if n.Kind != jqsyntax.NObject {
			return true
		}
		var keys []string
		for _, e := range n.Kids {
			if e.Kind != jqsyntax.NEntry || len(e.Kids) == 0 {
				continue
			}
			switch k := e.Kids[0]; {
			case k.Kind == jqsyntax.NKey:
				keys = append(keys, strings.TrimPrefix(k.Text, "$"))
			case k.Kind == jqsyntax.NString && len(k.Kids) == 1 && k.Kids[0].Kind == jqsyntax.NStrPart:
				keys = append(keys, k.Kids[0].Text)
			}
		}
		if len(keys) > 0 {
			sets = append(sets, keys)
		}
		return true
	})
	return sets
}

// marshal is gojq.Marshal with each object's keys in o's order.
func marshal(v any, o *shape.Order) ([]byte, error) {
	var b bytes.Buffer
	err := write(&b, v, o)
	return b.Bytes(), err
}

func write(b *bytes.Buffer, v any, o *shape.Order) error {
	switch v := v.(type) {
	case map[string]any:
		b.WriteByte('{')
		for i, k := range o.Keys(v) {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, err := gojq.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(kb)
			b.WriteByte(':')
			if err := write(b, v[k], o); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := write(b, e, o); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		j, err := gojq.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(j)
	}
	return nil
}
