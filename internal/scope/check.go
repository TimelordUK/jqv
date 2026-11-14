package scope

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/shape"
)

// Level is how sure a check is that the line is wrong.
type Level int

const (
	Note    Level = iota // worth knowing: a key some objects lack, an index past some arrays' ends
	Warning              // jq gives null where you surely meant something: a key never there
	Error                // jq fails on every value seen: a key asked of an array
)

func (l Level) String() string { return [...]string{"note", "warning", "error"}[l] }

// Diag is what a check found about a span of the line, From to To in runes.
type Diag struct {
	From, To int
	Level    Level
	Msg      string
	Suggest  []string // what to write instead: the nearest keys (`projectId`), or forms (`.projects[].x`)
}

// Check looks at every field and index in the line against what `.` is there (Q-046,
// DESIGN §22): a key no object at its place has (jq's quiet null), a key asked of arrays or
// scalars (jq's error), an index past the end of the arrays there. `.x?` isn't checked, nor
// anything whose input depends on a caller or whose path is already wrong further left.
func Check(line string, sh *Shapes) []Diag {
	toks := jqsyntax.Lex(line)
	rs := []rune(line)
	var ds []Diag
	for i, t := range toks {
		switch {
		case t.Kind == jqsyntax.Field && t.Text != ".":
			if n := nextCode(toks, i); n >= 0 && toks[n].Text == "?" {
				continue
			}
			sc := At(line, t.End)
			key := t.Text[1:]
			if !sc.Term.Typing || sc.Term.Key != key {
				continue
			}
			p, ok := sc.Value()
			if !ok {
				continue
			}
			where := string(rs[sc.Term.Pos:t.Pos])
			if d, ok := checkKey(sh.Of(p), key, where); ok {
				d.From, d.To = t.Pos, t.End
				ds = append(ds, d)
			}
		case t.Kind == jqsyntax.Bracket && t.Text == "[":
			idx, end, ok := index(toks, i)
			if !ok {
				continue
			}
			pv := prevCode(toks, i)
			if pv < 0 || !pathEnd(toks[pv]) {
				continue
			}
			if n := nextCode(toks, end); n >= 0 && toks[n].Text == "?" {
				continue
			}
			sc := At(line, t.Pos)
			p, ok := sc.Here()
			if !ok {
				continue
			}
			where := string(rs[sc.Term.Pos:t.Pos])
			if d, ok := checkIndex(sh.Of(p), idx, where); ok {
				d.From, d.To = t.Pos, toks[end].End
				ds = append(ds, d)
			}
		}
	}
	return ds
}

// Here is the program for all of the term at the cursor, the key being typed included:
// `.projects[0].na▮` on input `.[].employee` gives `.[].employee | .projects[0] | .na`.
func (s Scope) Here() (string, bool) {
	v, ok := s.Value()
	if !ok || !s.Term.Typing || s.Term.Key == "" {
		return v, ok
	}
	return render([]part{{text: v}, {text: "." + shape.KeyText(s.Term.Key)}}), true
}

// checkKey is what's wrong with asking key of what f describes, if anything. where is the
// path written before it (`.projects[0]`), for the message.
func checkKey(f Found, key, where string) (Diag, bool) {
	sh := f.Shape
	if f.Missing != "" || sh == nil || sh.Count == 0 {
		return Diag{}, false
	}
	if where == "" {
		where = "."
	}
	field := "." + shape.KeyText(key)
	objects := sh.Kinds[doc.Object]
	others := sh.Count - objects - sh.Kinds[doc.Null]
	switch {
	case objects == 0 && others > 0:
		k, only := sh.Only()
		if only && k == doc.Array {
			fix := []string{where + "[]" + field, where + "[0]" + field}
			if where == "." {
				fix = []string{".[]" + field, ".[0]" + field}
			}
			return Diag{Level: Error, Msg: fmt.Sprintf("%s: %s is an array here; a key is asked of its elements: %s", field, where, strings.Join(fix, " or ")), Suggest: fix}, true
		}
		return Diag{Level: Error, Msg: fmt.Sprintf("%s: %s is %s here, which has no keys", field, where, kinds(sh))}, true
	case objects == 0 || others > 0:
		return Diag{}, false // only nulls, or mixed: jq's answer depends on the value
	}
	c := sh.Find(key)
	them := "the object"
	if objects > 1 {
		them = fmt.Sprintf("the %d objects", objects)
	}
	if f.Cut {
		them += " of the first outputs"
	}
	if c == nil {
		var starting []string
		for _, k := range sh.Keys() {
			if strings.HasPrefix(k.Key, key) {
				starting = append(starting, k.Key)
			}
		}
		if len(starting) > 0 { // still being typed, most likely
			return Diag{Level: Note, Msg: fmt.Sprintf("%s: keys starting %q: %s", field, key, strings.Join(starting, " ")), Suggest: starting}, true
		}
		near := nearest(key, sh.Keys())
		msg := fmt.Sprintf("%s: no such key in %s here, so jq gives null", field, them)
		if len(near) > 0 {
			msg += "; did you mean ." + strings.Join(near, " or .") + "?"
		}
		return Diag{Level: Warning, Msg: msg, Suggest: near}, true
	}
	if c.Count < objects {
		return Diag{Level: Note, Msg: fmt.Sprintf("%s: missing in %d of %s here: null there", field, objects-c.Count, them)}, true
	}
	return Diag{}, false
}

// checkIndex is what's wrong with index n of what f describes: past the end of every array
// there, or of some. jq gives null for both, as quietly as for a missing key.
func checkIndex(f Found, n int, where string) (Diag, bool) {
	sh := f.Shape
	arrays := 0
	if sh != nil {
		arrays = sh.Kinds[doc.Array]
	}
	if f.Missing != "" || arrays == 0 || arrays != sh.Count {
		return Diag{}, false
	}
	if where == "" {
		where = "."
	}
	at := fmt.Sprintf("[%d]", n)
	size := func(l int) string {
		if l == 1 {
			return "1 element"
		}
		return strconv.Itoa(l) + " elements"
	}
	has := size(sh.MaxLen)
	if sh.MinLen != sh.MaxLen {
		has = fmt.Sprintf("%d..%d elements", sh.MinLen, sh.MaxLen)
	}
	all := "the array"
	if arrays > 1 {
		all = fmt.Sprintf("all %d arrays", arrays)
	}
	reach := n // the length an array needs for n to find something
	if n < 0 {
		reach = -n - 1
	}
	switch {
	case reach >= sh.MaxLen:
		msg := fmt.Sprintf("%s: %s has %s in %s here, so %s is always null", at, where, has, all, at)
		if f.Cut {
			msg = fmt.Sprintf("%s: %s has %s in the first outputs, so %s is null there", at, where, has, at)
		}
		return Diag{Level: Warning, Msg: msg}, true
	case reach >= sh.MinLen:
		return Diag{Level: Note, Msg: fmt.Sprintf("%s: %s has %s; %s is null in the %d of %d arrays that are shorter", at, where, has, at, sh.AtMost(reach), arrays)}, true
	}
	return Diag{}, false
}

// kinds names what was seen, for a message: "a string", "a number or a string".
func kinds(sh *shape.Shape) string {
	var ks []string
	for k, c := range sh.Kinds {
		if c > 0 && doc.Kind(k) != doc.Null {
			ks = append(ks, "a "+doc.Kind(k).String())
		}
	}
	return strings.Join(ks, " or ")
}

// nearest are the keys most like key: one containing it or contained in it, any case
// (`id` in `projectId`), then any within a few edits (`nmae`), at most three.
func nearest(key string, keys []*shape.Shape) []string {
	low := strings.ToLower(key)
	var in, close []string
	for _, k := range keys {
		kl := strings.ToLower(k.Key)
		switch {
		case strings.Contains(kl, low) || strings.Contains(low, kl):
			in = append(in, k.Key)
		case edits(low, kl) <= max(1, len([]rune(low))/3):
			close = append(close, k.Key)
		}
	}
	out := append(in, close...)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// edits is how many single-letter edits turn a into b: insert, delete, change, or swap two
// neighbours, so the commonest typo (`nmae`) is one edit, not two.
func edits(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// nextCode is the index of the first token after toks[i] that isn't space or a comment, or -1.
func nextCode(toks []jqsyntax.Token, i int) int {
	for i++; i < len(toks); i++ {
		if k := toks[i].Kind; k != jqsyntax.Space && k != jqsyntax.Comment {
			return i
		}
	}
	return -1
}

// index reads the integer index whose [ is toks[i]: `[9]`, `[-1]`; end is its ]'s index.
func index(toks []jqsyntax.Token, i int) (n, end int, ok bool) {
	j, sign := i+1, 1
	if j < len(toks) && toks[j].Text == "-" {
		j, sign = j+1, -1
	}
	if j+1 >= len(toks) || toks[j].Kind != jqsyntax.Number || toks[j+1].Text != "]" {
		return 0, 0, false
	}
	n, err := strconv.Atoi(toks[j].Text)
	if err != nil {
		return 0, 0, false
	}
	return sign * n, j + 1, true
}
