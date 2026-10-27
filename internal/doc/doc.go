// Package doc holds a JSON document as an ordered tree of nodes.
//
// encoding/json and gojq both decode objects into maps, which loses key order. A viewer must
// show the document as it was written, so doc builds its own tree from the token stream.
// Values handed to gojq come from Node.Value, which converts on demand.
package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Kind is the JSON type of a node.
type Kind uint8

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

func (k Kind) String() string {
	return [...]string{"null", "bool", "number", "string", "array", "object"}[k]
}

// Node is one value in the document. Object members carry their Key; array elements their Index.
type Node struct {
	Kind     Kind
	Key      string // member name, when Parent is an object
	Index    int    // position in Parent's children
	Raw      string // scalar text as written: number digits, true/false/null; decoded text for strings
	Children []*Node
	Parent   *Node
	Offset   int64 // byte offset of the value in the input
}

// IsContainer reports whether the node is an object or array.
func (n *Node) IsContainer() bool { return n.Kind == Array || n.Kind == Object }

// Path returns the node's jq path, e.g. `.items[1]."first name"`. The root is ".".
func (n *Node) Path() string {
	var parts []string
	for c := n; c.Parent != nil; c = c.Parent {
		if c.Parent.Kind == Array {
			parts = append(parts, "["+strconv.Itoa(c.Index)+"]")
		} else {
			parts = append(parts, keySegment(c.Key))
		}
	}
	if len(parts) == 0 {
		return "."
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
	}
	return b.String()
}

// keySegment renders an object key as a jq path step: `.name` when it is a plain
// identifier, `."odd key"` otherwise.
func keySegment(k string) string {
	if isIdent(k) {
		return "." + k
	}
	q, _ := json.Marshal(k)
	return "." + string(q)
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// Value converts the subtree to plain Go values (map[string]any, []any, json.Number, ...)
// as gojq expects. Numbers stay json.Number so large integers keep their precision.
func (n *Node) Value() any {
	switch n.Kind {
	case Null:
		return nil
	case Bool:
		return n.Raw == "true"
	case Number:
		return json.Number(n.Raw)
	case String:
		return n.Raw
	case Array:
		a := make([]any, len(n.Children))
		for i, c := range n.Children {
			a[i] = c.Value()
		}
		return a
	default:
		m := make(map[string]any, len(n.Children))
		for _, c := range n.Children {
			m[c.Key] = c.Value()
		}
		return m
	}
}

// Walk calls fn for n and every descendant in document order. Returning false skips a subtree.
func (n *Node) Walk(fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Parse reads exactly one JSON value. Trailing data other than whitespace is an error.
func Parse(r io.Reader) (*Node, error) {
	d := json.NewDecoder(r)
	d.UseNumber()
	root, err := parseValue(d, nil)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("offset %d: unexpected data after the top-level value", d.InputOffset())
	}
	return root, nil
}

// ParseBytes is Parse over a byte slice.
func ParseBytes(b []byte) (*Node, error) { return Parse(bytes.NewReader(b)) }

func parseValue(d *json.Decoder, parent *Node) (*Node, error) {
	off := d.InputOffset()
	tok, err := d.Token()
	if err == io.EOF {
		return nil, errors.New("empty input")
	}
	if err != nil {
		return nil, err
	}
	n := &Node{Parent: parent, Offset: off}
	switch t := tok.(type) {
	case nil:
		n.Kind, n.Raw = Null, "null"
	case bool:
		n.Kind, n.Raw = Bool, strconv.FormatBool(t)
	case json.Number:
		n.Kind, n.Raw = Number, string(t)
	case string:
		n.Kind, n.Raw = String, t
	case json.Delim:
		switch t {
		case '[':
			n.Kind = Array
			for d.More() {
				c, err := parseValue(d, n)
				if err != nil {
					return nil, err
				}
				c.Index = len(n.Children)
				n.Children = append(n.Children, c)
			}
		case '{':
			n.Kind = Object
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, err
				}
				c, err := parseValue(d, n)
				if err != nil {
					return nil, err
				}
				c.Key, c.Index = kt.(string), len(n.Children)
				n.Children = append(n.Children, c)
			}
		}
		if _, err := d.Token(); err != nil { // closing delimiter
			return nil, err
		}
	}
	return n, nil
}
