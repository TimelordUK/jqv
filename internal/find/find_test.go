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
