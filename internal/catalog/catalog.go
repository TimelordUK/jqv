// Package catalog is jq's own manual as data (Q-021, DESIGN §18): what each function and
// operator does, and the manual's examples, each a program, an input and the outputs jq gives.
// The REPL's :help reads it, and :try runs an example. manual.yml is jq's file as it is in
// jqlang/jq at the tag in Source (CC BY 3.0, see NOTICE), parsed on first use. The test runs
// every example through gojq and real jq, so the help can't say what jq doesn't do, and the
// places gojq differs are listed in Differs.
package catalog

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
	"go.yaml.in/yaml/v3"
)

// Source is where manual.yml came from. To move to a new jq, replace the file from that tag,
// run the tests, and update Differs from what they say.
const Source = "jq 1.8.2 manual (jqlang/jq docs/content/manual/v1.8/manual.yml, CC BY 3.0)"

//go:embed manual.yml
var manual []byte

// Entry is one topic: a function or operator (`trim`, `ltrim`, `rtrim`), or a section of the
// manual (Math, Regular expressions), which has a body and no examples of its own.
type Entry struct {
	Section  string
	Title    string // as the manual has it, with its backticks
	Body     string // markdown
	Names    []string
	Examples []Example
}

// Example is one of the manual's: Program run on Input gives Output, each a JSON text.
type Example struct {
	Program string
	Input   string
	Output  []string
	Differs string // why gojq gives something else, when it does (from Differs)
}

// Differs are the examples where gojq doesn't give what the manual says, by program, with why.
// TestExamples fails when this list and gojq disagree, in either direction.
var Differs = map[string]string{
	`[., tojson] == if have_decnum then [12345678909876543212345,"12345678909876543212345"] else [12345678909876543000000,"12345678909876543000000"] end`:                               "gojq has no have_decnum",
	`[1234567890987654321,-1234567890987654321 | tojson] == if have_decnum then ["1234567890987654321","-1234567890987654321"] else ["1234567890987654400","-1234567890987654400"] end`: "gojq has no have_decnum",
	`map([., . == 1]) | tojson == if have_decnum then "[[1,true],[1.000,true],[1.0,true],[1.00,true]]" else "[[1,true],[1,true],[1,true],[1,true]]" end`:                                "gojq has no have_decnum",
	`. as $big | [$big, $big + 1] | map(. > 10000000000000000000000000000000) | . == if have_decnum then [true, false] else [false, false] end`:                                         "gojq has no have_decnum",
	`try error("\($__loc__)") catch .`:               "gojq has no $__loc__",
	`.[] | test("a b c # spaces are ignored"; "ix")`: `gojq has no "x" (extended) regex flag`,
	`match(["foo", "ig"])`:                           "gojq doesn't take the [regex, flags] array form",
	`splits(",? *"; "n")`:                            `gojq has no "n" regex flag`,
}

// NotInGojq are functions the manual documents that gojq, jqv's engine, doesn't define, with
// what to do instead. TestNamesAgainstGojq keeps the list true both ways.
var NotInGojq = map[string]string{
	"have_decnum":          "gojq keeps big integers exact, and has no decimal-number mode to ask about",
	"have_literal_numbers": "gojq has no literal-number mode to ask about",
	"input_filename":       "gojq leaves it to its command line; jqv runs on one document",
	"input_line_number":    "jqv runs on one document; there are no input lines",
	"keys_unsorted":        "gojq doesn't keep key order in values; keys gives them sorted (Q-010)",
	"stderr":               "gojq leaves stderr to its command line; the library jqv uses has none",
}

// Catalog is the manual, indexed by name.
type Catalog struct {
	Entries []*Entry
	byName  map[string][]*Entry
}

var (
	once   sync.Once
	loaded *Catalog
	err    error
)

// Load parses the manual the first time it is asked for.
func Load() (*Catalog, error) {
	once.Do(func() { loaded, err = parse(manual) })
	return loaded, err
}

type yManual struct {
	Sections []struct {
		Title   string
		Body    string
		Entries []struct {
			Title    string
			Body     string
			Examples []struct {
				Program string
				Input   string
				Output  []string
			}
		}
	}
}

func parse(b []byte) (*Catalog, error) {
	var m yManual
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	c := &Catalog{byName: map[string][]*Entry{}}
	for _, s := range m.Sections {
		c.add(&Entry{Section: s.Title, Title: s.Title, Body: s.Body})
		for _, y := range s.Entries {
			e := &Entry{Section: s.Title, Title: y.Title, Body: y.Body}
			for _, x := range y.Examples {
				e.Examples = append(e.Examples, Example{x.Program, x.Input, x.Output, Differs[x.Program]})
			}
			e.Names = names(y.Title)
			c.add(e)
		}
	}
	return c, nil
}

func (c *Catalog) add(e *Entry) {
	c.Entries = append(c.Entries, e)
	for _, n := range e.Names {
		c.byName[n] = append(c.byName[n], e)
	}
}

var (
	codeSpan = regexp.MustCompile("`([^`]+)`")
	leadName = regexp.MustCompile(`^\$?[A-Za-z_][A-Za-z0-9_]*`)
)

// names are what a title answers to: each code span's leading name (`map(f)` is map,
// `import RelativePathString as NAME` is import), or the span itself for an operator (`|`,
// `?//`, `.[]`). `... as $identifier | ...` is also `as`.
func names(title string) []string {
	var ns []string
	for _, m := range codeSpan.FindAllStringSubmatch(title, -1) {
		n, span := "", m[1]
		switch {
		case leadName.MatchString(span):
			n = leadName.FindString(span)
		case strings.HasPrefix(span, "... as "):
			n = "as"
		default:
			n = span
		}
		if !slices.Contains(ns, n) { // `splits(regex)`, `splits(regex; flags)`
			ns = append(ns, n)
		}
	}
	return ns
}

// Plain is a title without its backticks: "trim, ltrim, rtrim".
func Plain(title string) string { return strings.ReplaceAll(title, "`", "") }

// Lookup gives the entries for a name (`trim`, `|`, `first`), else for a title or section
// (`math`, `regular expressions`, any case). exact is false when nothing has that name and the
// entries are a search of titles and bodies instead, titles first.
func (c *Catalog) Lookup(q string) (es []*Entry, exact bool) {
	q = strings.TrimSpace(q)
	if es := c.byName[q]; len(es) > 0 {
		return es, true
	}
	lq := strings.ToLower(q)
	for _, e := range c.Entries {
		if strings.ToLower(Plain(e.Title)) == lq {
			return []*Entry{e}, true
		}
	}
	var inTitle, inBody []*Entry
	for _, e := range c.Entries {
		switch {
		case strings.Contains(strings.ToLower(Plain(e.Title)), lq):
			inTitle = append(inTitle, e)
		case strings.Contains(strings.ToLower(e.Body), lq):
			inBody = append(inBody, e)
		}
	}
	return append(inTitle, inBody...), false
}

// Section gives a section's own entry (its body) and the entries under it.
func (c *Catalog) Section(title string) []*Entry {
	var es []*Entry
	for _, e := range c.Entries {
		if e.Section == title {
			es = append(es, e)
		}
	}
	return es
}

// Sections are the section titles in the manual's order.
func (c *Catalog) Sections() []string {
	var ts []string
	for _, e := range c.Entries {
		if e.Title == e.Section {
			ts = append(ts, e.Title)
		}
	}
	return ts
}

// Values are the example's input and outputs as values, as gojq gives them.
func (x Example) Values() (in any, out []any, err error) {
	if in, err = decode(x.Input); err != nil {
		return nil, nil, err
	}
	for _, o := range x.Output {
		v, err := decode(o)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, v)
	}
	return in, out, nil
}

// Same says whether got are the outputs the manual gives (as values, so 1.0 is 1 and key order
// doesn't matter).
func (x Example) Same(got []any) bool {
	_, want, err := x.Values()
	if err != nil || len(got) != len(want) {
		return false
	}
	for i := range got {
		if !reflect.DeepEqual(norm(got[i]), norm(want[i])) {
			return false
		}
	}
	return true
}

func decode(s string) (any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return norm(v), nil
}

// norm takes a value through gojq's JSON and back, so numbers compare alike however written.
func norm(v any) any {
	b, err := gojq.Marshal(v)
	if err != nil {
		return v
	}
	var o any
	json.Unmarshal(b, &o)
	return o
}

// Names are every name in the catalog, sorted.
func (c *Catalog) Names() []string {
	ns := make([]string, 0, len(c.byName))
	for n := range c.byName {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}
