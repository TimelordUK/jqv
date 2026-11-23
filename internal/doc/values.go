package doc

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// FromValues is a tree over the values a run gave, for looking at what a stage receives
// (DESIGN §28.2): an array holding each value, so a stream of records is the root's children.
// gojq's objects are maps, so their key order comes from keys.
func FromValues(vs []any, keys func(map[string]any) []string) *Node {
	root := &Node{Kind: Array}
	seq := 0
	var add func(parent *Node, key string, v any)
	add = func(parent *Node, key string, v any) {
		seq++
		n := &Node{Key: key, Index: len(parent.Children), Parent: parent, Seq: seq}
		parent.Children = append(parent.Children, n)
		switch v := v.(type) {
		case nil:
			n.Kind, n.Raw = Null, "null"
		case bool:
			n.Kind, n.Raw = Bool, strconv.FormatBool(v)
		case string:
			n.Kind, n.Raw = String, v
		case map[string]any:
			n.Kind = Object
			for _, k := range keys(v) {
				add(n, k, v[k])
			}
		case []any:
			n.Kind = Array
			for _, e := range v {
				add(n, "", e)
			}
		default: // int, float64, *big.Int: numbers as JSON writes them
			n.Kind = Number
			if b, err := json.Marshal(v); err == nil {
				n.Raw = string(b)
			} else {
				n.Raw = fmt.Sprint(v)
			}
		}
	}
	for _, v := range vs {
		add(root, "", v)
	}
	return root
}
