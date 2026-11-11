package assist

import (
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/editor"
)

// completions writes what Tab offers at ▮: the span replaced, then each candidate's insert.
func completions(s string) (span string, ins []string, cs []editor.Candidate) {
	i := strings.Index(s, "▮")
	line := strings.Replace(s, "▮", "", 1)
	from, to, cs := Complete(line, len([]rune(s[:i])))
	for _, c := range cs {
		ins = append(ins, c.Insert)
	}
	return string([]rune(line)[from:to]), ins, cs
}

func TestComplete(t *testing.T) {
	for _, c := range []struct {
		in, span string
		first    []string // the first candidates, in order
	}{
		{":▮", ":", []string{":push", ":pop", ":reset"}},
		{":p▮", ":p", []string{":push", ":pop", ":program", ":pp"}},
		{":tr▮ee .a", ":tree", []string{":tree", ":try"}},
		{":help comb▮", "comb", []string{"combinations"}},
		{":help regular e▮", "regular e", []string{"regular expressions"}},
		{":help Mat▮", "Mat", []string{"match", "math"}}, // any case
		{"sel▮", "sel", []string{"select("}},             // takes arguments: its (
		{".[] | len▮", "len", []string{"length"}},        // bare
		{".[] | len▮gth", "length", []string{"length"}},  // the whole word goes
		{"map▮", "map", []string{"map(", "map_values("}}, // the exact one first
		{"red▮", "red", []string{"reduce"}},              // keywords too
		{"$r▮", "$r", []string{"$root"}},
		{". as $people | $p▮", "$p", []string{"$people"}},         // bound in the line
		{"def peeps: .[].people[]; pe▮", "pe", []string{"peeps"}}, // the line's defs
		{"def older(n): select(.age > n); old▮", "old", []string{"older("}},
		{":tree se▮", "se", []string{"select("}}, // jq in a command's argument
		{".sel▮", "", nil},                       // a field: keys, later
		{`"sel▮"`, "", nil},                      // in a string: nothing
		{".a # sel▮", "", nil},                   // in a comment: nothing
		{":! ba▮", "", nil},                      // not jq
		{"zzz▮", "zzz", nil},
	} {
		span, ins, _ := completions(c.in)
		if len(ins) > len(c.first) {
			ins = ins[:len(c.first)]
		}
		if span != c.span || strings.Join(ins, " ") != strings.Join(c.first, " ") {
			t.Errorf("%q: span %q, %q; want %q, %q", c.in, span, ins, c.span, c.first)
		}
	}
}

func TestCompleteHints(t *testing.T) {
	for _, c := range []struct{ in, hint string }{
		{":pu▮", ":push [expr]  :p  make the last line (or expr) a stage: . is now what it gives"},
		{"ltri▮", "trim, ltrim, rtrim: "},
		{"$ro▮", "the whole document"},
		{"def older(n): select(.age > n); old▮", "def older(n)  (defined in this line)"},
	} {
		_, _, cs := completions(c.in)
		if len(cs) == 0 || !strings.HasPrefix(cs[0].Hint, c.hint) {
			t.Errorf("%q: hints %+v, want one starting %q", c.in, cs, c.hint)
		}
	}
}
