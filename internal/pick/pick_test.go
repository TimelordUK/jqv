package pick

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/query"
)

func load(t *testing.T, name string) *doc.Node {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// at finds the node at a jq path, as the cursor would be on it.
func at(t *testing.T, root *doc.Node, p string) *doc.Node {
	t.Helper()
	var found *doc.Node
	root.Walk(func(n *doc.Node) bool {
		if found == nil && n.Path() == p {
			found = n
		}
		return found == nil
	})
	if found == nil {
		t.Fatalf("no node at %s", p)
	}
	return found
}

func TestWrite(t *testing.T) {
	for _, c := range []struct {
		f    Form
		want string
	}{
		{Form{}, ""},
		{Form{Columns: []Column{{"Job", []string{"Job"}}}}, ".[] | {Job}"},
		{Form{Grain: []string{"orders"}, Columns: []Column{
			{"id", []string{"id"}},
			{"symbol", []string{"instrument", "symbol"}},
		}}, ".orders[] | {id, symbol: .instrument.symbol}"},
		{Form{Columns: []Column{{"@timestamp", []string{"@timestamp"}}}}, `.[] | {"@timestamp"}`},
		{Form{Grain: []string{"a b"}, Columns: []Column{{"x y", []string{"p", "x y"}}}}, `."a b"[] | {"x y": .p."x y"}`},
		{Form{Within: [][]string{{"prizes"}}, Grain: []string{"laureates"}, Columns: []Column{
			{"id", []string{"id"}}, {"firstname", []string{"firstname"}},
		}}, ".prizes[].laureates[]? | {id, firstname}"},
		{Form{Within: [][]string{nil}, Grain: nil, Columns: []Column{{"x", []string{"x"}}}}, ".[][]? | {x}"},
		{Form{Within: [][]string{nil, {"a"}}, Grain: []string{"b", "c"}, Columns: []Column{{"x", []string{"x"}}}}, ".[].a[]?.b.c[]? | {x}"},
		{Form{Columns: []Column{{"if", []string{"if"}}, {"end", []string{"r", "end"}}}}, `.[] | {"if", "end": .r."end"}`},
	} {
		if got := Write(c.f); got != c.want {
			t.Errorf("Write(%v) = %q, want %q", c.f, got, c.want)
		}
	}
}

// Read and Write are each other's inverse on the form (DESIGN §26): what a gesture reads back
// is what was written, and spacing aside, the text is what Write gives.
func TestReadWrite(t *testing.T) {
	for _, text := range []string{
		".[] | {Job}",
		".[] | {Job, Status, DurationSec}",
		".orders[] | {id, symbol: .instrument.symbol, qty}",
		`.[] | {"@timestamp", level}`,
		`."a b"[] | {"x y": .p."x y"}`,
		`.[] | {"if", "end": .r."end"}`,
		".data.items[] | {id, name: .meta.name}",
		".[] | {id, \"instrument.id\": .instrument.id}",
	} {
		f, err := Read(text)
		if err != nil {
			t.Errorf("Read(%q): %v", text, err)
			continue
		}
		if got := Write(f); got != text {
			t.Errorf("Write(Read(%q)) = %q", text, got)
		}
	}
	// Spacing is not the form; the text is the same program.
	f, err := Read(".orders[]|{ id ,symbol:.instrument.symbol }")
	if err != nil || Write(f) != ".orders[] | {id, symbol: .instrument.symbol}" {
		t.Errorf("spacing: %v %q", err, Write(f))
	}
}

// Anything that isn't exactly the form is refused, never rewritten into it.
func TestReadRefuses(t *testing.T) {
	for _, text := range []string{
		".[]",
		"{Job}",
		".[] | {Job} | .Job",
		".[] | select(.x) | {Job}",
		".[0] | {Job}",
		".[]? | {Job}",
		".[] | {Job: .Job[]}",
		".[] | {Job: (.a | .b)}",
		".[] | {Job: .a?}",
		".[] | {(.k): .v}",
		".[] | {$x}",
		`.[] | {"a\(1)": .a}`,
		".[] | {}",
		".[] | {Job}[]",
		".[][] | {Job}",
		".[] | {Job: 1}",
		".[] as $x | {Job}",
		"not jq ((",
	} {
		if f, err := Read(text); err == nil {
			t.Errorf("Read(%q) = %+v, want an error", text, f)
		}
	}
}

func TestLift(t *testing.T) {
	tc := load(t, "teamcity_runs.json")
	orders := load(t, "orders.json")
	nobel := load(t, "nobel_prize.json")
	for _, c := range []struct {
		root  *doc.Node
		at    string
		grain []string
		path  []string
		err   string
	}{
		{nobel, ".prizes[0].laureates[1].firstname", []string{"laureates"}, []string{"firstname"}, ""},
		{tc, ".[3].Job", nil, []string{"Job"}, ""},
		{orders, ".orders[1].instrument.symbol", []string{"orders"}, []string{"instrument", "symbol"}, ""},
		{orders, ".orders[0].qty", []string{"orders"}, []string{"qty"}, ""},
		{orders, ".source", nil, nil, "inside an array of records"},
		{orders, ".paging.page", nil, nil, "inside an array of records"},
		{orders, ".orders[0]", nil, nil, "not a whole object"},
		{orders, ".orders[0].tags[1]", nil, nil, "not an array's own value"},
		{orders, ".orders[0].fills[1].qty", []string{"fills"}, []string{"qty"}, ""},
	} {
		p, err := Lift(at(t, c.root, c.at))
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("Lift(%s) = %v, %v; want error with %q", c.at, p, err, c.err)
			}
			continue
		}
		if err != nil || !slices.Equal(p.Grain, c.grain) || !slices.Equal(p.Path, c.path) {
			t.Errorf("Lift(%s) = %+v, %v; want %v %v", c.at, p, err, c.grain, c.path)
		}
	}
}

func TestToggle(t *testing.T) {
	orders := load(t, "orders.json")
	lift := func(p string) Pick {
		t.Helper()
		pk, err := Lift(at(t, orders, p))
		if err != nil {
			t.Fatal(err)
		}
		return pk
	}
	var f Form
	steps := []struct {
		at   string
		want string
	}{
		{".orders[0].id", ".orders[] | {id}"},
		{".orders[2].qty", ".orders[] | {id, qty}"},
		{".orders[1].instrument.symbol", ".orders[] | {id, qty, symbol: .instrument.symbol}"},
		{".orders[1].id", ".orders[] | {qty, symbol: .instrument.symbol}"}, // again unpicks
		{".orders[0].id", ".orders[] | {qty, symbol: .instrument.symbol, id}"},
	}
	for _, s := range steps {
		var err error
		f, err = Toggle(f, lift(s.at))
		if err != nil {
			t.Fatalf("%s: %v", s.at, err)
		}
		if got := Write(f); got != s.want {
			t.Fatalf("after %s: %q, want %q", s.at, got, s.want)
		}
		// Every step goes through the text, as a gesture does.
		if f, err = Read(Write(f)); err != nil {
			t.Fatalf("Read back after %s: %v", s.at, err)
		}
	}
	if !f.Has(lift(".orders[2].instrument.symbol")) || f.Has(lift(".orders[0].price")) {
		t.Error("Has: wrong marks")
	}
	for _, p := range []string{".orders[0].qty", ".orders[0].instrument.symbol", ".orders[0].id"} {
		f, _ = Toggle(f, lift(p))
	}
	if !f.Empty() || Write(f) != "" {
		t.Errorf("all unpicked: %+v", f)
	}
}

// A name that is taken takes its parents' in front (GRID §4), and the text still reads back.
func TestToggleNames(t *testing.T) {
	f := Form{Grain: []string{"orders"}}
	for _, p := range [][]string{{"id"}, {"instrument", "id"}, {"x", "instrument", "id"}} {
		f, _ = Toggle(f, Pick{Grain: []string{"orders"}, Path: p})
	}
	want := `.orders[] | {id, "instrument.id": .instrument.id, "x.instrument.id": .x.instrument.id}`
	if got := Write(f); got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	if _, err := Read(want); err != nil {
		t.Error(err)
	}
}

func TestToggleOtherGrain(t *testing.T) {
	f, _ := Toggle(Form{}, Pick{Grain: []string{"orders"}, Path: []string{"id"}})
	if _, err := Toggle(f, Pick{Grain: []string{"trades"}, Path: []string{"id"}}); err == nil ||
		!strings.Contains(err.Error(), ".trades[]") {
		t.Errorf("want a refusal naming .trades[], got %v", err)
	}
}

// GRID §10's promise, on real documents: every pick gives a program that runs, with one row
// per record of the grain and the columns in pick order.
func TestPicksRun(t *testing.T) {
	for _, c := range []struct {
		file string
		at   []string
	}{
		{"teamcity_runs.json", []string{".[0].Job", ".[0].Status", ".[0].Number", ".[0].Agent"}},
		{"orders.json", []string{".orders[0].id", ".orders[0].instrument.symbol", ".orders[0].notes", ".orders[0].qty"}},
		{"users_100.json", nil},
		{"nobel_prize.json", []string{".prizes[0].laureates[0].id", ".prizes[0].laureates[0].surname", ".prizes[3].laureates[0].share"}},
		{"orders.json", []string{".orders[0].fills[0].qty"}},
	} {
		root := load(t, c.file)
		var f Form
		for _, p := range c.at {
			pk, err := Lift(at(t, root, p))
			if err != nil {
				t.Fatal(err)
			}
			if f, err = Toggle(f, pk); err != nil {
				t.Fatal(err)
			}
		}
		if f.Empty() {
			continue
		}
		r := query.Run(Write(f), root.Value(), nil, query.Options{})
		if r.Err != nil {
			t.Fatalf("%s: %s: %v", c.file, Write(f), r.Err)
		}
		records := query.Run("["+iterate(append(slices.Clone(f.Within), f.Grain))+"] | length", root.Value(), nil, query.Options{})
		if n := records.Values[0]; len(r.Values) != toInt(n) {
			t.Errorf("%s: %d rows, want %v", c.file, len(r.Values), n)
		}
		for _, v := range r.Values {
			if m := v.(map[string]any); len(m) != len(f.Columns) {
				t.Errorf("%s: row %v, want %d columns", c.file, m, len(f.Columns))
			}
		}
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return -1
}

// Read(Write(f)) == f for random forms, odd keys included (DESIGN §26).
func TestRoundTripRandom(t *testing.T) {
	keys := []string{"id", "Job", "@timestamp", "a b", "if", "end", "x.y", `q"t`, "ünï", "_1", "1a", "$x", ""}
	rng := rand.New(rand.NewPCG(1, 2))
	pickKeys := func(n int) []string {
		ks := make([]string, n)
		for i := range ks {
			ks[i] = keys[rng.IntN(len(keys))]
		}
		return ks
	}
	for range 2000 {
		var f Form
		f.Grain = pickKeys(rng.IntN(3))
		if len(f.Grain) == 0 {
			f.Grain = nil
		}
		for range rng.IntN(3) {
			w := pickKeys(rng.IntN(3))
			if len(w) == 0 {
				w = nil
			}
			f.Within = append(f.Within, w)
		}
		for range 1 + rng.IntN(4) {
			f, _ = Toggle(f, Pick{Within: f.Within, Grain: f.Grain, Path: pickKeys(1 + rng.IntN(3))})
		}
		if f.Empty() {
			continue
		}
		text := Write(f)
		g, err := Read(text)
		if err != nil {
			t.Fatalf("Read(%s): %v", text, err)
		}
		if !reflect.DeepEqual(f, g) {
			t.Fatalf("round trip of %s:\n got %+v\nwant %+v", text, g, f)
		}
	}
}

// The bar's pretty form fits its width where it can, and is still the program.
func TestPretty(t *testing.T) {
	f := Form{Within: [][]string{{"prizes"}}, Grain: []string{"laureates"}}
	for _, n := range []string{"id", "firstname", "surname", "motivation", "share"} {
		f.Columns = append(f.Columns, Column{n, []string{n}})
	}
	f.Columns = append(f.Columns, Column{"born", []string{"dates", "born"}})
	if got := Pretty(f, 200); len(got) != 1 || got[0] != Write(f) {
		t.Errorf("wide: %q", got)
	}
	got := Pretty(f, 30)
	want := []string{
		".prizes[].laureates[]? | {",
		"    id, firstname, surname,",
		"    motivation, share,",
		"    born: .dates.born",
		"}",
	}
	if !slices.Equal(got, want) {
		t.Errorf("narrow:\n%s", strings.Join(got, "\n"))
	}
	if g, err := Read(strings.Join(got, " ")); err != nil || !reflect.DeepEqual(g, f) {
		t.Errorf("pretty doesn't read back: %v", err)
	}
}
