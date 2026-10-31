package find

import (
	"reflect"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
)

const sample = `{"orderId":"ORD-1","Status":"Filled","orders":[{"orderId":"ord-2","qty":2500},{"orderId":"ORD-3","note":"see ORD-1"}],"flag":true}`

func paths(ns []*doc.Node) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.Path())
	}
	return out
}

func root(t *testing.T) *doc.Node {
	t.Helper()
	r, err := doc.ParseBytes([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSmartCase(t *testing.T) {
	cases := []struct {
		pat  string
		want []string
	}{
		{"ord-", []string{".orderId", ".orders[0].orderId", ".orders[1].orderId", ".orders[1].note"}},   // lower: ignore case, values only ("orderid" keys don't contain "ord-")
		{"ORD-", []string{".orderId", ".orders[1].orderId", ".orders[1].note"}},                         // capital: exact
		{`ORD-\c`, []string{".orderId", ".orders[0].orderId", ".orders[1].orderId", ".orders[1].note"}}, // \c forces ignore case
		{`status\C`, []string{}}, // \C forces exact
		{"status", []string{".Status"}},
		{"orderid", []string{".orderId", ".orders[0].orderId", ".orders[1].orderId"}}, // keys
		{"orders", []string{".orders"}},                                               // a container matches on its key
		{"2500", []string{".orders[0].qty"}},
		{"true", []string{".flag"}},
		{"", []string{}},
	}
	r := root(t)
	for _, c := range cases {
		if got := paths(All(r, Parse(c.pat))); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q:\n got %q\nwant %q", c.pat, got, c.want)
		}
	}
}

func TestArrayIndexNotSearched(t *testing.T) {
	r, _ := doc.ParseBytes([]byte(`["a","b"]`))
	if got := All(r, Parse("1")); len(got) != 0 {
		t.Fatalf("matched %q", paths(got))
	}
}

func TestQuoted(t *testing.T) {
	const js = `{"AE":"AE","MsgType":"AE","other":"TAKE","ae":"ae","n":2,"s":"2","flag":"true","on":true,"AEX":[1],"Long":"AE and more"}`
	r, err := doc.ParseBytes([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		pat  string
		want []string
	}{
		{`"AE"`, []string{".AE", ".MsgType"}},                 // the whole key or string, not TAKE or AEX
		{`"ae"`, []string{".AE", ".MsgType", ".ae"}},          // smartcase as ever
		{`"ae"\C`, []string{".ae"}},                           // and \C
		{`"AE`, []string{".AE", ".MsgType", ".AEX", ".Long"}}, // no closing quote yet: starts with
		{`"2"`, []string{".s"}},                               // a string, not the number
		{`"true"`, []string{".flag"}},                         // nor the bool
		{`"`, []string{}},                                     // nothing to look for yet
		{`AE"`, []string{}},                                   // a quote elsewhere is just text
	}
	for _, c := range cases {
		if got := paths(All(r, Parse(c.pat))); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.pat, got, c.want)
		}
	}
}

// Filtering the last matches must give what a full walk gives, for every pair of patterns.
func TestRefine(t *testing.T) {
	r := root(t)
	cases := []struct {
		from, to string
		refines  bool
	}{
		{"o", "or", true},
		{"rd", "ord", true}, // not only by adding at the end
		{"ord", "or", false},
		{"ord", "ORD", false}, // the capital turns off ignoring case
		{"ORD", "ORD-", true},
		{"ord", `ORD\c`, true}, // the same text under the same case rule
		{`ord\c`, `ORD-\c`, true},
		{"ord", `"ord`, true},   // typing the quotes around the last pattern narrows it
		{`"ord`, `"ord-`, true}, // starts with, longer
		{`"ord`, `"ord-1"`, true},
		{`"ord-1`, `"ord"`, false},
		{`"ord-1"`, `"ord-1"`, true},
		{`"ord-1"`, `"ord-1`, false}, // deleting the closing quote widens it
		{`"ord-1"`, `"ord-1"x`, false},
		{`"ord`, "ord", false},
		{`"ord`, "ord-", false},
		{"", "o", false},
		{"o", "", false},
	}
	for _, c := range cases {
		from, to := Parse(c.from), Parse(c.to)
		if got := to.Refines(from); got != c.refines {
			t.Errorf("%q refines %q = %v", c.to, c.from, got)
		}
		if c.refines {
			if got, want := paths(Filter(All(r, from), to)), paths(All(r, to)); !reflect.DeepEqual(got, want) {
				t.Errorf("%q then %q:\n got %q\nwant %q", c.from, c.to, got, want)
			}
		}
	}
}

func TestIndex(t *testing.T) {
	r := root(t)
	m := All(r, Parse("ord-"))
	for i, n := range m {
		if got := Index(m, n); got != i {
			t.Errorf("Index(%s) = %d, want %d", n.Path(), got, i)
		}
	}
	if got := Index(m, r); got != -1 {
		t.Errorf("Index of a non-match = %d", got)
	}
}

func TestNextWraps(t *testing.T) {
	r := root(t)
	m := All(r, Parse("ord-")) // .orderId, .orders[0].orderId, .orders[1].orderId, .orders[1].note
	cases := []struct {
		from    *doc.Node
		forward bool
		want    string
		wrapped bool
	}{
		{r, true, ".orderId", false},
		{m[0], true, ".orders[0].orderId", false},
		{m[3], true, ".orderId", true},
		{m[0], false, ".orders[1].note", true},
		{m[2], false, ".orders[0].orderId", false},
		{r.Children[2], true, ".orders[0].orderId", false}, // from a non-match in between
		{r.Children[2], false, ".orderId", false},
		{r.Children[3], true, ".orderId", true}, // .flag, after the last match
	}
	for _, c := range cases {
		i, w := Next(m, c.from, c.forward)
		if m[i].Path() != c.want || w != c.wrapped {
			t.Errorf("from %s fwd=%v: got %s wrapped=%v, want %s wrapped=%v", c.from.Path(), c.forward, m[i].Path(), w, c.want, c.wrapped)
		}
	}
	if i, _ := Next(nil, r, true); i != -1 {
		t.Fatal("no matches should give -1")
	}
}
