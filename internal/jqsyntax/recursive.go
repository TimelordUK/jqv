package jqsyntax

import (
	"maps"
	"strconv"
	"unicode"
)

// Recursion is what Recursive finds in a program (Q-052).
type Recursion struct {
	Defs     []RecursiveDef
	Builtins [][2]int // where recurse, walk and .. are called, in runes: builtins that recurse
}

// RecursiveDef is a def that calls itself: where its name is, and each call to itself.
type RecursiveDef struct {
	Def   *Node
	Name  [2]int
	Calls [][2]int
}

// recursing are the builtins that recurse, by name and arity: recurse and recurse(f) walk
// down a value, walk(f) applies f from the leaves up.
var recursing = map[string]bool{"recurse/0": true, "recurse/1": true, "recurse/2": true, "walk/1": true}

// Recursive finds the defs in text that call themselves, with jq's scoping: a def is seen in
// its own body and after it, and an inner def or a closure parameter of the same name and
// arity hides it. A call from a def nested inside counts (g's inner h calling g). Calls
// between defs (f calls g, g calls f) are not found. Half-typed text gives what its tree has.
func Recursive(text string) Recursion {
	rs := []rune(text)
	var r Recursion
	found := map[*Node]int{} // a def's index in r.Defs
	var walk func(n *Node, env map[string]*Node, in []*Node)
	walk = func(n *Node, env map[string]*Node, in []*Node) {
		if n == nil {
			return
		}
		switch n.Kind {
		case NDefs:
			env = maps.Clone(env)
			for _, k := range n.Kids {
				if k.Kind != NDef {
					walk(k, env, in)
					continue
				}
				env[key(k.Text, params(k))] = k // seen in its own body and after
				body := maps.Clone(env)
				for _, p := range k.Kids {
					if p.Kind == NParam && len(p.Text) > 0 && p.Text[0] != '$' {
						body[key(p.Text, 0)] = nil // a closure parameter hides a def of its name
					}
				}
				for _, b := range k.Kids {
					if b.Kind != NParam {
						walk(b, body, append(in[:len(in):len(in)], k))
					}
				}
			}
			return
		case NCall:
			name := []rune(n.Text)
			span := [2]int{n.Pos, min(n.Pos+len(name), n.End)}
			d, bound := env[key(n.Text, len(n.Kids))]
			switch {
			case d != nil && contains(in, d):
				i, ok := found[d]
				if !ok {
					i = len(r.Defs)
					found[d] = i
					r.Defs = append(r.Defs, RecursiveDef{Def: d, Name: defName(rs, d)})
				}
				r.Defs[i].Calls = append(r.Defs[i].Calls, span)
			case !bound && recursing[key(n.Text, len(n.Kids))]:
				r.Builtins = append(r.Builtins, span)
			}
		case NRecurse:
			r.Builtins = append(r.Builtins, [2]int{n.Pos, n.End})
		}
		for _, k := range n.Kids {
			walk(k, env, in)
		}
	}
	walk(Parse(text), map[string]*Node{}, nil)
	return r
}

func key(name string, arity int) string { return name + "/" + strconv.Itoa(arity) }

func params(d *Node) int {
	c := 0
	for _, k := range d.Kids {
		if k.Kind == NParam {
			c++
		}
	}
	return c
}

func contains(ns []*Node, n *Node) bool {
	for _, m := range ns {
		if m == n {
			return true
		}
	}
	return false
}

// defName is where d's name is: after `def` and the space.
func defName(rs []rune, d *Node) [2]int {
	i := min(d.Pos+3, len(rs))
	for i < len(rs) && unicode.IsSpace(rs[i]) {
		i++
	}
	j := i
	for j < len(rs) && (rs[j] == '_' || unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) { // a def's name has no ::
		j++
	}
	return [2]int{i, j}
}
