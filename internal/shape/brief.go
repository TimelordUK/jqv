package shape

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
)

// Brief is one level of s in a line of about width runes, for a frame of the scope stack
// (DESIGN §22): its types and counts, array lengths, the scalars as :structure says them,
// and an object's keys in the order first seen, as many as fit, then how many more. An
// array's elements follow "of", so `array len 6 of object ×6 {employee}` needs no `.[]`.
// Nothing deeper is shown, so FIX4.4 can't flood it: `:structure` is for depth.
func Brief(s *Shape, width int) string {
	return brief(s, width, 0)
}

func brief(s *Shape, width, depth int) string {
	if s == nil || s.Count == 0 {
		return "nothing"
	}
	var kinds []string
	for k, c := range s.Kinds {
		if c > 0 {
			kinds = append(kinds, fmt.Sprintf("%s ×%d", doc.Kind(k), c))
		}
	}
	if _, ok := s.Only(); ok && s.Count == 1 {
		kinds[0] = strings.Fields(kinds[0])[0]
	}
	parts := []string{strings.Join(kinds, " | ")}
	if l := lengths(s); l != "" {
		parts = append(parts, l)
	}
	if k, ok := s.Only(); !ok || k != doc.Null { // `null ×6` says it all
		if v := scalars(s); v != "" {
			parts = append(parts, v)
		}
	}
	if s.Mixed() {
		parts = append(parts, "⚠ mixed")
	}
	out := strings.Join(parts, "  ")
	if s.Kinds[doc.Object] > 0 {
		out += " " + keyList(s, width-len([]rune(out))-1)
	}
	if s.elem != nil && depth < 2 {
		out += " of " + brief(s.elem, width-len([]rune(out))-4, depth+1)
	}
	return out
}

// keyList is s's keys in braces, as many as fit in width runes, then `… N more`; a key an
// object here sometimes lacks is marked `?`.
func keyList(s *Shape, width int) string {
	keys := s.Keys()
	if len(keys) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, c := range keys {
		k := KeyText(c.Key)
		if c.Count < s.Kinds[doc.Object] {
			k += "?"
		}
		more := ""
		if i < len(keys)-1 {
			more = " … " + strconv.Itoa(len(keys)-i) + " more}"
		}
		if i > 0 && len([]rune(b.String()))+1+len([]rune(k))+len([]rune(more)) > width {
			b.WriteString(" … " + strconv.Itoa(len(keys)-i) + " more")
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
	}
	b.WriteByte('}')
	return b.String()
}

// KeyText is a key as a field after a dot: bare when it is an identifier, quoted when not
// (`name`, `"$id"`).
func KeyText(k string) string {
	if isIdent(k) {
		return k
	}
	q, _ := json.Marshal(k)
	return string(q)
}

// Keys are the object keys seen here, in the order first seen.
func (s *Shape) Keys() []*Shape {
	var ks []*Shape
	for _, c := range s.Children {
		if !c.Elem {
			ks = append(ks, c)
		}
	}
	return ks
}

// Elements is the shape of the arrays' elements here, or nil.
func (s *Shape) Elements() *Shape { return s.elem }
