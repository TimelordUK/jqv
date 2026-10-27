package doc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Node {
	t.Helper()
	n, err := ParseBytes([]byte(s))
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return n
}

func TestKeyOrderKept(t *testing.T) {
	root := mustParse(t, `{"zeta":1,"alpha":2,"mid":3}`)
	var keys []string
	for _, c := range root.Children {
		keys = append(keys, c.Key)
	}
	if want := []string{"zeta", "alpha", "mid"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestPaths(t *testing.T) {
	root := mustParse(t, `{"items":[{"qty":1},{"first name":"x","_id9":2}],"":0}`)
	var got []string
	root.Walk(func(n *Node) bool { got = append(got, n.Path()); return true })
	want := []string{
		".",
		".items",
		".items[0]",
		".items[0].qty",
		".items[1]",
		`.items[1]."first name"`,
		".items[1]._id9",
		`.""`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths\n got %q\nwant %q", got, want)
	}
}

func TestScalars(t *testing.T) {
	root := mustParse(t, `[null,true,false,12345678901234567890,-1.5e3,"a\"bé"]`)
	want := []struct {
		k   Kind
		raw string
	}{{Null, "null"}, {Bool, "true"}, {Bool, "false"}, {Number, "12345678901234567890"}, {Number, "-1.5e3"}, {String, `a"bé`}}
	for i, w := range want {
		c := root.Children[i]
		if c.Kind != w.k || c.Raw != w.raw {
			t.Errorf("[%d] = %v %q, want %v %q", i, c.Kind, c.Raw, w.k, w.raw)
		}
	}
}

func TestValueRoundTrip(t *testing.T) {
	src := `{"a":[1,2.50,{"b":null}],"c":"x","d":true,"big":12345678901234567890}`
	got, _ := json.Marshal(mustParse(t, src).Value())
	var a, b any
	json.Unmarshal([]byte(src), &a)
	json.Unmarshal(got, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("round trip changed value: %s", got)
	}
	if !strings.Contains(string(got), "12345678901234567890") {
		t.Fatalf("big integer lost precision: %s", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{``, `   `, `{`, `[1,]`, `{"a"}`, `{"a":1} 2`, `nul`} {
		if _, err := ParseBytes([]byte(s)); err == nil {
			t.Errorf("Parse(%q): want error", s)
		}
	}
}

// FuzzParse: never panics, and anything accepted means the same as encoding/json says.
func FuzzParse(f *testing.F) {
	for _, s := range []string{`{}`, `[]`, `{"a":[1,{"b":null}]}`, `"x"`, `1e5`, `[true,false]`, `{"a":1}x`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		n, err := ParseBytes(b)
		if err != nil {
			return
		}
		if !json.Valid(b) {
			t.Fatalf("accepted invalid JSON %q", b)
		}
		got, err := json.Marshal(n.Value())
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var want, have any
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.UseNumber()
		d.Decode(&want)
		d = json.NewDecoder(strings.NewReader(string(got)))
		d.UseNumber()
		d.Decode(&have)
		if !reflect.DeepEqual(want, have) {
			t.Fatalf("value differs for %q: got %s", b, got)
		}
	})
}
