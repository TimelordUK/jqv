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
	"strconv"
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

// notes are jqv's own entries in the manual's shape (jqv.yml): examples for what the manual
// only lists, such as the math functions. They are checked as the manual's are.
//
//go:embed jqv.yml
var notes []byte

// Entry is one topic: a function or operator (`trim`, `ltrim`, `rtrim`), or a section of the
// manual (Math, Regular expressions), which has a body and no examples of its own.
type Entry struct {
	Section  string
	Title    string // as the manual has it, with its backticks
	Body     string // markdown
	Names    []string
	Aliases  []string // words it is found by that aren't jq names (`modulo`), from jqv.yml
	Examples []Example
}

// Example is one of the manual's: Program run on Input gives Output, each a JSON text.
type Example struct {
	Program string
	Input   string
	Output  []string
	Differs string // why gojq gives something else, when it does (from Differs)
	Notes   bool   // from jqv's notes (jqv.yml), not jq's manual
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
	aliases map[string][]*Entry // jqv.yml's other words for entries; not names, so not offered
}

var (
	once   sync.Once
	loaded *Catalog
	err    error
)

// Load parses the manual the first time it is asked for.
func Load() (*Catalog, error) {
	once.Do(func() { loaded, err = parse(manual, notes) })
	return loaded, err
}

type yManual struct {
	Sections []struct {
		Title   string
		Body    string
		Entries []struct {
			Title    string
			Body     string
			Aliases  []string // other names it answers to: jqv.yml's `modulo` for `%`
			Examples []struct {
				Program string
				Input   string
				Output  []string
			}
		}
	}
}

// parse reads the manual, then jqv's notes, each in the manual's YAML shape. The notes come
// before names are taken from the manual's text, so a function they cover (`remainder`)
// gets their entry rather than only the section that lists it.
func parse(docs ...[]byte) (*Catalog, error) {
	c := &Catalog{byName: map[string][]*Entry{}, aliases: map[string][]*Entry{}}
	for i, b := range docs {
		var m yManual
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		for _, s := range m.Sections {
			c.add(&Entry{Section: s.Title, Title: s.Title, Body: s.Body})
			for _, y := range s.Entries {
				e := &Entry{Section: s.Title, Title: y.Title, Body: y.Body}
				for _, x := range y.Examples {
					e.Examples = append(e.Examples, Example{Program: x.Program, Input: x.Input, Output: x.Output,
						Differs: Differs[x.Program], Notes: i > 0})
				}
				e.Names, e.Aliases = names(y.Title), y.Aliases
				c.add(e)
			}
		}
	}
	c.textNames()
	return c, nil
}

var (
	textName = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)(\\([^`]*\\))?`")
	format   = regexp.MustCompile("`(@[a-z0-9]+)`")
)

// textNames gives the entries whose titles name nothing (Math, Dates, Convert to/from JSON,
// SQL-Style Operators) the functions their text names in backticks: `sin`, `strftime`,
// `tojson`, `INDEX(stream; f)`. Only names gojq defines count, so a word in backticks isn't
// taken for a function, and a name that has an entry of its own (`floor`) keeps it. Formats
// (`@csv`, `@base64`) are names too, for :help @csv.
func (c *Catalog) textNames() {
	defined := builtins()
	for _, e := range c.Entries {
		if len(e.Names) > 0 {
			continue
		}
		for _, m := range textName.FindAllStringSubmatch(e.Body, -1) {
			if n := m[1]; defined[n] && len(c.byName[n]) == 0 {
				e.Names = append(e.Names, n)
				c.byName[n] = []*Entry{e}
			}
		}
		for _, m := range format.FindAllStringSubmatch(e.Body, -1) {
			if n := m[1]; n != "@foo" && len(c.byName[n]) == 0 { // @foo is the text's "some format"
				e.Names = append(e.Names, n)
				c.byName[n] = []*Entry{e}
			}
		}
	}
}

// builtins are the names of the functions gojq defines.
func builtins() map[string]bool {
	ns := map[string]bool{}
	q, err := gojq.Parse(`builtins[] | split("/")[0]`)
	if err != nil {
		return ns
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return ns
	}
	it := code.Run(nil)
	for {
		v, ok := it.Next()
		if !ok {
			return ns
		}
		if s, ok := v.(string); ok {
			ns[s] = true
		}
	}
}

// keywords are names in the manual's titles that are syntax, not functions.
var keywords = map[string]bool{"reduce": true, "foreach": true, "if": true, "try": true,
	"label": true, "import": true, "include": true, "module": true, "def": true, "as": true,
	"and": true, "or": true}

// Functions are the names of jq's functions, sorted: not operators (`|`), paths (`.foo`),
// variables (`$ENV`) or keywords (`reduce`, `and`). The Math section's are in.
func (c *Catalog) Functions() []string {
	var fs []string
	for _, n := range c.Names() {
		if leadName.MatchString(n) && leadName.FindString(n) == n && !strings.HasPrefix(n, "$") && !keywords[n] {
			fs = append(fs, n)
		}
	}
	return fs
}

func (c *Catalog) add(e *Entry) {
	c.Entries = append(c.Entries, e)
	for _, n := range e.Names {
		c.byName[n] = append(c.byName[n], e)
	}
	for _, a := range e.Aliases {
		c.aliases[a] = append(c.aliases[a], e)
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
	if es := append(slices.Clone(c.byName[q]), c.aliases[strings.ToLower(q)]...); len(es) > 0 {
		return slices.Compact(es), true
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

// Arities are the numbers of arguments each function gojq defines takes (`remainder` 2,
// `range` 1, 2 and 3), for the hint when a call has the wrong number.
func Arities() map[string][]int {
	as := map[string][]int{}
	q, err := gojq.Parse(`builtins[]`)
	if err != nil {
		return as
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return as
	}
	it := code.Run(nil)
	for {
		v, ok := it.Next()
		if !ok {
			break
		}
		s, _ := v.(string)
		name, n, ok := strings.Cut(s, "/")
		if a, err := strconv.Atoi(n); ok && err == nil {
			as[name] = append(as[name], a)
		}
	}
	for _, a := range as {
		slices.Sort(a)
	}
	return as
}

// Signature is how the manual (or jqv's notes) writes a call of name with n arguments:
// `remainder(x; y)`, `ltrimstr(str)`; "" when no title has one.
func (c *Catalog) Signature(name string, n int) string {
	for _, e := range append(slices.Clone(c.byName[name]), c.Entries...) {
		for _, m := range codeSpan.FindAllStringSubmatch(e.Title, -1) {
			span := m[1]
			if leadName.FindString(span) != name {
				continue
			}
			args := 0
			if open := strings.Index(span, "("); open >= 0 {
				args = strings.Count(span[open:], ";") + 1
			}
			if args == n {
				return span
			}
		}
	}
	return ""
}
