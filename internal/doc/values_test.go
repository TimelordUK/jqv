package doc

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"testing"
)

func TestFromValues(t *testing.T) {
	vs := []any{
		map[string]any{"b": 1, "a": json.Number("12.0"), "c": []any{true, nil, "x"}},
		"s",
		2.5,
	}
	keys := func(m map[string]any) []string {
		ks := make([]string, 0, len(m))
		for k := range m {
			ks = append(ks, k)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(ks)))
		return ks
	}
	root := FromValues(vs, keys)
	if root.Kind != Array || len(root.Children) != 3 {
		t.Fatalf("root %v with %d children", root.Kind, len(root.Children))
	}
	rec := root.Children[0]
	var got []string
	for _, c := range rec.Children {
		got = append(got, c.Key)
	}
	if !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Errorf("keys %v, want the order keys gave", got)
	}
	if p := rec.Children[0].Children[2].Path(); p != "[0].c[2]" && p != ".[0].c[2]" {
		t.Errorf("path %q", p)
	}
	want := []any{
		map[string]any{"b": json.Number("1"), "a": json.Number("12.0"), "c": []any{true, nil, "x"}},
		"s",
		json.Number("2.5"),
	}
	if v := root.Value(); !reflect.DeepEqual(v, want) {
		t.Errorf("Value() = %#v", v)
	}
	seen := map[int]bool{}
	root.Walk(func(n *Node) bool {
		if n != root && seen[n.Seq] {
			t.Errorf("Seq %d twice", n.Seq)
		}
		seen[n.Seq] = true
		return true
	})
}
