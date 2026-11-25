package assist

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/shape"
)

// carryOn is flattening a sub-object a key at a time (Q-079, owner 2026-11-25): after
// `did: .department.id, ▮` the other keys of .department, named with the entry's prefix
// (`dname: .department.name`), and an offer that takes them all, or nil. The prefix is the
// entry's key less its field's name, any case: did → d, department_id → department_,
// departmentId → department with each name capitalised. A key that ends in only the first
// letters of the name is an abbreviation, and the others are cut the same: after
// `dmn: .department.manager.name` comes `dmc: .department.manager.contact`. A key that ends in
// none of it gives no prefix. Keys whose path or name the literal already has are left out.
func carryOn(s *bench.Session, expr string, cur int, program string) (cs []editor.Candidate, all *editor.Candidate, from string) {
	obj := literal(expr, cur)
	if obj == nil {
		return nil, nil, ""
	}
	rs := []rune(expr)
	var prev *jqsyntax.Node
	for _, e := range obj.Kids {
		if e.Kind == jqsyntax.NEntry && e.End <= cur && !(e.Pos <= cur && cur <= e.End) {
			prev = e
		}
	}
	if prev == nil || len(prev.Kids) != 2 {
		return nil, nil, ""
	}
	key, val := entryKey(prev.Kids[0]), prev.Kids[1]
	segs := fieldPath(val)
	if key == "" || len(segs) < 2 {
		return nil, nil, ""
	}
	x := segs[len(segs)-1]
	last := val.Kids[len(val.Kids)-1]
	path := strings.TrimSpace(string(rs[val.Pos:last.Pos])) // .department
	of := path
	if program != "." {
		of = program + " | " + path
	}
	sh := s.Shapes().Of(of).Shape
	if sh == nil || sh.Kinds[doc.Object] == 0 {
		return nil, nil, ""
	}

	// the longest end of the key that starts the field's name: all of it (did, id) or its
	// first letters (dmn, name), which the other keys are then cut to as well
	prefix, abbrev, capital := "", 0, false
	for l := min(len(key), len(x)); l > 0; l-- {
		if strings.EqualFold(key[len(key)-l:], x[:l]) {
			prefix = key[:len(key)-l]
			if l < len(x) {
				abbrev = l
			}
			r, _ := utf8.DecodeRuneInString(key[len(key)-l:])
			capital = unicode.IsUpper(r) && !strings.HasPrefix(x, string(r))
			break
		}
	}
	short := func(k string) string {
		if abbrev > 0 && len(k) > abbrev {
			return k[:abbrev]
		}
		return k
	}
	seen := map[string]int{} // cut names two keys share go in whole
	for _, k := range sh.Keys() {
		seen[short(k.Key)]++
	}
	have, used := literalKeys(expr, cur), map[string]bool{}
	for _, e := range obj.Kids {
		if e.Kind == jqsyntax.NEntry && len(e.Kids) == 2 {
			used[strings.TrimSpace(string(rs[e.Kids[1].Pos:e.Kids[1].End]))] = true
		}
	}
	var texts []string
	for _, k := range sh.Keys() {
		p := path + "." + shape.KeyText(k.Key)
		name := k.Key
		if seen[short(name)] == 1 {
			name = short(name)
		}
		if capital {
			r, n := utf8.DecodeRuneInString(name)
			name = string(unicode.ToUpper(r)) + name[n:]
		}
		name = prefix + name
		if k.Key == x || used[p] || have[name] {
			continue
		}
		text := shorthand(name) + ": " + p
		texts = append(texts, text)
		cs = append(cs, editor.Candidate{Insert: text, Hint: text + "  " + shape.Brief(k, 80) + "  · after " + key})
	}
	if len(cs) > 1 {
		all = &editor.Candidate{Insert: strings.Join(texts, ", "), Show: "… the rest of " + path,
			Hint: "the rest of " + path + ", " + plural(len(cs), "key") + ": " + strings.Join(texts, ", ")}
	}
	return cs, all, path
}

// fieldPath is a value's keys when it is only fields from .: `.department.id` is
// [department id]; nil for anything else (an index, a call, parentheses).
func fieldPath(n *jqsyntax.Node) []string {
	kids := []*jqsyntax.Node{n}
	if n.Kind == jqsyntax.NPostfix {
		kids = n.Kids
	}
	var segs []string
	for _, k := range kids {
		switch {
		case k.Kind == jqsyntax.NField && len(k.Text) > 1:
			segs = append(segs, k.Text[1:])
		case k.Kind == jqsyntax.NField && len(k.Kids) == 1 && plainString(k.Kids[0]) != "":
			segs = append(segs, plainString(k.Kids[0]))
		default:
			return nil
		}
	}
	return segs
}

// plainString is a string node's text when it has no interpolation or escapes; "" otherwise.
func plainString(n *jqsyntax.Node) string {
	if n.Kind != jqsyntax.NString {
		return ""
	}
	var b strings.Builder
	for _, p := range n.Kids {
		if p.Kind != jqsyntax.NStrPart {
			return ""
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

// entryKey is an entry's key as a name: bare, $x as x, or a plain string; "" otherwise.
func entryKey(k *jqsyntax.Node) string {
	switch k.Kind {
	case jqsyntax.NKey:
		return strings.TrimPrefix(k.Text, "$")
	case jqsyntax.NString:
		return plainString(k)
	}
	return ""
}
