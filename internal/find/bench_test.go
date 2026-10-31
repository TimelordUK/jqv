package find

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
)

// orders builds an order-shaped document of about mb megabytes, as DESIGN §11 measured.
func orders(tb testing.TB, mb int) *doc.Node {
	tb.Helper()
	var b strings.Builder
	b.WriteString(`{"orders":[`)
	for i := 0; b.Len() < mb<<20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"ORD-%d","status":"FILLED","qty":%d,"instrument":{"symbol":"VOD.L","currency":"GBP","venue":"LSE"},"trader":"Olivia Smith","note":"Allocated to Book %d"}`, i, i%1000, i%50)
	}
	b.WriteString(`]}`)
	root, err := doc.ParseBytes([]byte(b.String()))
	if err != nil {
		tb.Fatal(err)
	}
	return root
}

// BenchmarkFilter is the cost of each keystroke after the first, narrowing the hits of "o", which
// here is the worst case: it matches nearly every record.
func BenchmarkFilter(b *testing.B) {
	root := orders(b, 50)
	hits, q := All(root, Parse("o")), Parse("ol")
	for b.Loop() {
		Filter(hits, q)
	}
}

// BenchmarkAll is the cost of one keystroke of incremental search over a whole document.
func BenchmarkAll(b *testing.B) {
	for _, mb := range []int{10, 50} {
		root := orders(b, mb)
		for _, p := range []string{"o", "olivia", "zzz", "Olivia"} {
			q := Parse(p)
			b.Run(fmt.Sprintf("%dMB/%s", mb, p), func(b *testing.B) {
				for b.Loop() {
					All(root, q)
				}
			})
		}
	}
}
