package doc

import (
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// ParseYAML reads YAML into the same tree as Parse (C-013, DESIGN §12), so nothing after the
// load knows the difference. Key order is kept, anchors and merge keys (`<<`) are expanded, and
// a file of several documents becomes an array of them (empty ones, as after a trailing `---`,
// are left out). Scalars take YAML's types (`3` is a
// number, `true` a boolean, `~` null); values JSON has no form for (timestamps, .inf, binary)
// stay strings as written. Offsets are not kept: YAML has lines, not byte offsets.
func ParseYAML(r io.Reader) (*Node, error) {
	d := yaml.NewDecoder(r)
	var docs []*yaml.Node
	seen := 0
	for {
		var y yaml.Node
		err := d.Decode(&y)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		seen++
		if !blank(&y) { // a trailing `---` starts a document with nothing in it
			docs = append(docs, &y)
		}
	}
	c := &yconv{budget: maxYAMLNodes}
	var root *Node
	var err error
	switch len(docs) {
	case 0:
		if seen == 0 {
			return nil, errors.New("empty input")
		}
		root = &Node{Kind: Null, Raw: "null"} // only comments, or a lone `---`
	case 1:
		root, err = c.node(docs[0], nil, 0)
	default:
		root = &Node{Kind: Array}
		for i, y := range docs {
			var n *Node
			if n, err = c.node(y, root, 0); err != nil {
				break
			}
			n.Index = i
			root.Children = append(root.Children, n)
		}
	}
	if err != nil {
		return nil, err
	}
	seq := 0
	root.Walk(func(n *Node) bool { n.Seq = seq; seq++; return true })
	return root, nil
}

// blank is a document with nothing written in it (not even `~`).
func blank(y *yaml.Node) bool {
	if len(y.Content) == 0 {
		return true
	}
	c := y.Content[0]
	return c.Kind == yaml.ScalarNode && c.Value == "" && c.Tag == "!!null" && c.Style == 0
}

// maxYAMLNodes bounds what aliases may expand to: a few lines of anchors can name a billion
// values ("billion laughs"), and the tree holds every one.
const maxYAMLNodes = 20_000_000

type yconv struct{ budget int }

func (c *yconv) node(y *yaml.Node, parent *Node, depth int) (*Node, error) {
	if depth > 10_000 {
		return nil, fmt.Errorf("line %d: nested too deeply", y.Line)
	}
	if c.budget--; c.budget < 0 {
		return nil, fmt.Errorf("line %d: aliases expand to more than %d values", y.Line, maxYAMLNodes)
	}
	switch y.Kind {
	case yaml.DocumentNode:
		if len(y.Content) == 0 {
			return &Node{Kind: Null, Raw: "null", Parent: parent}, nil
		}
		return c.node(y.Content[0], parent, depth+1)
	case yaml.AliasNode:
		return c.node(y.Alias, parent, depth+1)
	case yaml.SequenceNode:
		n := &Node{Kind: Array, Parent: parent}
		for _, e := range y.Content {
			k, err := c.node(e, n, depth+1)
			if err != nil {
				return nil, err
			}
			k.Index = len(n.Children)
			n.Children = append(n.Children, k)
		}
		return n, nil
	case yaml.MappingNode:
		n := &Node{Kind: Object, Parent: parent}
		return n, c.members(n, y, depth, map[string]bool{})
	}
	return scalar(y, parent), nil
}

// members adds a mapping's pairs to n in order. A merge key (`<<: *base`, or a list of them)
// puts the merged mapping's pairs where it stands, less any key the mapping sets itself or an
// earlier merge gave; seen holds the keys n has or will have.
func (c *yconv) members(n *Node, y *yaml.Node, depth int, seen map[string]bool) error {
	for i := 0; i+1 < len(y.Content); i += 2 {
		if k := y.Content[i]; !isMerge(k) {
			seen[k.Value] = true
		}
	}
	for i := 0; i+1 < len(y.Content); i += 2 {
		k, v := y.Content[i], y.Content[i+1]
		if isMerge(k) {
			srcs := []*yaml.Node{v}
			if deref(v).Kind == yaml.SequenceNode {
				srcs = deref(v).Content
			}
			for _, s := range srcs {
				if s = deref(s); s.Kind != yaml.MappingNode {
					return fmt.Errorf("line %d: << merges a mapping, not a %s", s.Line, kindName(s))
				}
				sub := &Node{Kind: Object}
				if err := c.members(sub, s, depth+1, map[string]bool{}); err != nil {
					return err
				}
				for _, m := range sub.Children {
					if !seen[m.Key] {
						seen[m.Key] = true
						m.Parent, m.Index = n, len(n.Children)
						n.Children = append(n.Children, m)
					}
				}
			}
			continue
		}
		if k = deref(k); k.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a %s as a key; JSON keys are strings", k.Line, kindName(k))
		}
		m, err := c.node(v, n, depth+1)
		if err != nil {
			return err
		}
		m.Key, m.Index = k.Value, len(n.Children)
		n.Children = append(n.Children, m)
	}
	return nil
}

func isMerge(k *yaml.Node) bool { return k.Kind == yaml.ScalarNode && k.ShortTag() == "!!merge" }

func deref(y *yaml.Node) *yaml.Node {
	for y.Kind == yaml.AliasNode {
		y = y.Alias
	}
	return y
}

func kindName(y *yaml.Node) string {
	switch y.Kind {
	case yaml.SequenceNode:
		return "list"
	case yaml.MappingNode:
		return "mapping"
	}
	return "scalar"
}

var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$`)

// scalar gives a YAML scalar its JSON kind. Numbers JSON can write as they are keep their
// digits (so big integers keep their precision); 0x1F, 1_000 and +1.5 are written as JSON.
func scalar(y *yaml.Node, parent *Node) *Node {
	n := &Node{Kind: String, Raw: y.Value, Parent: parent}
	switch y.ShortTag() {
	case "!!null":
		n.Kind, n.Raw = Null, "null"
	case "!!bool":
		var b bool
		if y.Decode(&b) == nil {
			n.Kind, n.Raw = Bool, strconv.FormatBool(b)
		}
	case "!!int", "!!float":
		if jsonNumber.MatchString(y.Value) {
			n.Kind = Number
			break
		}
		var v any
		if y.Decode(&v) != nil {
			break
		}
		switch v := v.(type) {
		case int:
			n.Kind, n.Raw = Number, strconv.Itoa(v)
		case int64:
			n.Kind, n.Raw = Number, strconv.FormatInt(v, 10)
		case uint64:
			n.Kind, n.Raw = Number, strconv.FormatUint(v, 10)
		case float64:
			if !math.IsInf(v, 0) && !math.IsNaN(v) {
				n.Kind, n.Raw = Number, strconv.FormatFloat(v, 'g', -1, 64)
			}
		}
	}
	return n
}
