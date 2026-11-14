// Package shape summarises a document's structure (Q-040, DESIGN §21): every place in it
// with array indices folded to `[]`, so `.orders[3].id` and `.orders[7].id` are one place,
// `.orders[].id`. Each place keeps the types seen there, how many times, array lengths and
// the spread of its scalars. It is what `:structure` prints, and what key completion (Q-007),
// the schema view (V-005) and recipe suggestions will read.
//
// A Shape is built from the ordered doc tree, so keys come in the order they were written, or
// from gojq values, whose objects are maps: their keys come sorted, as gojq prints them.
package shape

import (
	"encoding/json"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
)

// MaxDistinct is how many distinct scalar values a place remembers before it stops counting
// and says only that there are more.
const MaxDistinct = 1000

// Shape is one place in the document: the root, an object key, or the elements of arrays.
type Shape struct {
	Key   string // the object key; "" for the root and for elements
	Elem  bool   // the elements of the arrays here ([])
	Count int    // values seen here
	Kinds [6]int // of them, how many of each doc.Kind

	MinLen, MaxLen int         // arrays: fewest and most elements
	lens           map[int]int // arrays: how many of each length
	MinNum, MaxNum float64

	distinct map[string]int // scalar text (strings quoted) → times seen, up to MaxDistinct
	order    []string       // the distinct values in the order first seen
	overflow bool           // more distinct values than MaxDistinct

	Children []*Shape // keys in first-seen order, then [] if any arrays were seen here
	elem     *Shape
	keys     map[string]*Shape
}

// FromNode is the shape of a document tree.
func FromNode(n *doc.Node) *Shape {
	s := &Shape{}
	s.addNode(n)
	return s
}

// FromValues is the shape of a stream of gojq values, each counted at the root. gojq's
// objects are maps, with no order, so their keys are put in the order of the document they
// came from (KeyOrder); order may be nil, and then keys are sorted.
func FromValues(vs []any, order *Order) *Shape {
	s := &Shape{}
	for _, v := range vs {
		s.addValue(v, order)
	}
	return s
}

// Order is how a document wrote its keys, to put gojq's maps back in that order.
type Order struct {
	sets map[string][]string // an object's keys sorted and joined → as first written
	rank map[string]int      // every key by first appearance, for objects jq built
}

// KeyOrder reads a document's key order. An object that passed through jq unchanged (`.[]`,
// a slice, a select) gets its keys exactly as written; one jq built gets them by where each
// key first appears in the document, then sorted.
func KeyOrder(n *doc.Node) *Order {
	o := &Order{sets: map[string][]string{}, rank: map[string]int{}}
	var walk func(*doc.Node)
	walk = func(n *doc.Node) {
		if n.Kind == doc.Object && len(n.Children) > 1 {
			keys := make([]string, len(n.Children))
			for i, c := range n.Children {
				keys[i] = c.Key
			}
			sig := signature(keys)
			if _, ok := o.sets[sig]; !ok {
				o.sets[sig] = keys
			}
		}
		for _, c := range n.Children {
			if n.Kind == doc.Object {
				if _, ok := o.rank[c.Key]; !ok {
					o.rank[c.Key] = len(o.rank)
				}
			}
			walk(c)
		}
	}
	walk(n)
	return o
}

// signature is a key set as one string, the same whatever order the keys come in.
func signature(keys []string) string {
	ks := append([]string(nil), keys...)
	sort.Strings(ks)
	return strings.Join(ks, string(rune(0)))
}

// keys are m's keys in the order o says.
func (o *Order) keys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if o != nil {
		if ks, ok := o.sets[signature(keys)]; ok {
			return ks
		}
	}
	var rank map[string]int
	if o != nil {
		rank = o.rank
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, oki := rank[keys[i]]
		rj, okj := rank[keys[j]]
		if oki != okj {
			return oki
		}
		if oki && ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// Mixed reports whether more than one type was seen here. null beside one other type is not
// counted as mixed: an optional value is normal.
func (s *Shape) Mixed() bool {
	n := 0
	for k, c := range s.Kinds {
		if c > 0 && doc.Kind(k) != doc.Null {
			n++
		}
	}
	return n > 1
}

// Only is the one type seen here, and false when there were several (nulls included).
func (s *Shape) Only() (doc.Kind, bool) {
	var only doc.Kind
	n := 0
	for k, c := range s.Kinds {
		if c > 0 {
			only, n = doc.Kind(k), n+1
		}
	}
	return only, n == 1
}

// Distinct is how many different scalar values were seen, and whether there were more than
// MaxDistinct (then the count stops there).
func (s *Shape) Distinct() (int, bool) { return len(s.order), s.overflow }

// Values are the distinct scalar values in the order first seen, as JSON text, with how often
// each came.
func (s *Shape) Values() []ValueCount {
	vs := make([]ValueCount, len(s.order))
	for i, v := range s.order {
		vs[i] = ValueCount{v, s.distinct[v]}
	}
	return vs
}

// Top are the n most frequent values, most frequent first, ties in the order first seen.
func (s *Shape) Top(n int) []ValueCount {
	vs := s.Values()
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].N > vs[j].N })
	if len(vs) > n {
		vs = vs[:n]
	}
	return vs
}

// ValueCount is a scalar as JSON text and how many times it was seen.
type ValueCount struct {
	Text string
	N    int
}

// Find is the shape at a generalised path given as steps: a key, or "[]" for elements.
func (s *Shape) Find(steps ...string) *Shape {
	for _, st := range steps {
		if s == nil {
			return nil
		}
		if st == "[]" {
			s = s.elem
		} else {
			s = s.keys[st]
		}
	}
	return s
}

func (s *Shape) child(key string) *Shape {
	if s.keys == nil {
		s.keys = map[string]*Shape{}
	}
	c := s.keys[key]
	if c == nil {
		c = &Shape{Key: key}
		s.keys[key] = c
		// keys before [], so a place both object and array lists its keys first
		if s.elem != nil {
			s.Children = append(s.Children[:len(s.Children)-1], c, s.elem)
		} else {
			s.Children = append(s.Children, c)
		}
	}
	return c
}

func (s *Shape) elems() *Shape {
	if s.elem == nil {
		s.elem = &Shape{Elem: true}
		s.Children = append(s.Children, s.elem)
	}
	return s.elem
}

func (s *Shape) see(k doc.Kind) {
	s.Count++
	s.Kinds[k]++
}

func (s *Shape) length(n int) {
	if s.lens == nil {
		s.lens = map[int]int{}
	}
	s.lens[n]++
	if s.Kinds[doc.Array] == 1 || n < s.MinLen {
		s.MinLen = n
	}
	if s.Kinds[doc.Array] == 1 || n > s.MaxLen {
		s.MaxLen = n
	}
}

func (s *Shape) scalar(k doc.Kind, text string) {
	if k == doc.Number {
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			if s.Kinds[doc.Number] == 1 || f < s.MinNum {
				s.MinNum = f
			}
			if s.Kinds[doc.Number] == 1 || f > s.MaxNum {
				s.MaxNum = f
			}
		}
	}
	if s.distinct == nil {
		s.distinct = map[string]int{}
	}
	if _, ok := s.distinct[text]; ok {
		s.distinct[text]++
		return
	}
	if len(s.order) >= MaxDistinct {
		s.overflow = true
		return
	}
	s.distinct[text] = 1
	s.order = append(s.order, text)
}

func (s *Shape) addNode(n *doc.Node) {
	s.see(n.Kind)
	switch n.Kind {
	case doc.Object:
		for _, c := range n.Children {
			s.child(c.Key).addNode(c)
		}
	case doc.Array:
		s.length(len(n.Children))
		for _, c := range n.Children {
			s.elems().addNode(c)
		}
	case doc.String:
		s.scalar(n.Kind, strconv.Quote(n.Raw))
	default:
		s.scalar(n.Kind, n.Raw)
	}
}

func (s *Shape) addValue(v any, order *Order) {
	switch v := v.(type) {
	case map[string]any:
		s.see(doc.Object)
		for _, k := range order.keys(v) {
			s.child(k).addValue(v[k], order)
		}
	case []any:
		s.see(doc.Array)
		s.length(len(v))
		for _, e := range v {
			s.elems().addValue(e, order)
		}
	case string:
		s.see(doc.String)
		s.scalar(doc.String, strconv.Quote(v))
	case bool:
		s.see(doc.Bool)
		s.scalar(doc.Bool, strconv.FormatBool(v))
	case nil:
		s.see(doc.Null)
		s.scalar(doc.Null, "null")
	case int:
		s.see(doc.Number)
		s.scalar(doc.Number, strconv.Itoa(v))
	case float64:
		s.see(doc.Number)
		s.scalar(doc.Number, strconv.FormatFloat(v, 'g', -1, 64))
	case json.Number:
		s.see(doc.Number)
		s.scalar(doc.Number, v.String())
	case *big.Int:
		s.see(doc.Number)
		s.scalar(doc.Number, v.String())
	default:
		s.see(doc.Null)
	}
}
