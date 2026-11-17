// Package program reads a jq program into units (DESIGN §23): the header comment, each
// top-level def with its doc comment, and main's stages. Units are spans over the text, not
// a rebuilt program: they tile it, so joined they give it back exactly, and editing one
// replaces its span and nothing else.
package program

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
)

// Kind is what a Unit is.
type Kind int

const (
	Header    Kind = iota // the comment block at the top, before the first unit's doc
	Directive             // module, import or include
	Def                   // a top-level def
	Stage                 // a stage of main: what a top-level | or an as's | separates; a def written in main is one too
)

var kindNames = [...]string{"header", "directive", "def", "stage"}

func (k Kind) String() string { return kindNames[k] }

// Unit is a piece of the program, Pos to End in runes. A comment that starts its own line
// belongs to the unit below it: its doc (Doc to From, the lines just above it) and any ruler
// further up (# ---- helpers ----). A comment after code on its line, and the blank lines
// after, belong to the unit above. Its code is From to To; a stage after the first starts
// at the | before it.
type Unit struct {
	Kind     Kind
	Name     string // a def's name and parameters, members($depth); a bind's patterns, as $fields
	Pos, End int
	Doc      int // where its doc comment starts; From when it has none
	From, To int
	Node     *jqsyntax.Node // the def, the bind, or the stage's tree; nil for a header
}

// Program is a program's text, its tree and its units.
type Program struct {
	Text  string
	Tree  *jqsyntax.Node
	Units []Unit
	rs    []rune
}

// Parse reads text into units. It never fails: what doesn't parse is a stage of its own.
func Parse(text string) *Program {
	p := &Program{Text: text, Tree: jqsyntax.Parse(text), rs: []rune(text)}
	b := &builder{rs: p.rs, ts: jqsyntax.Lex(text)}
	b.top(p.Tree)
	for i := range b.us {
		u := &b.us[i]
		u.Doc = b.docStart(u.From)
		if i == 0 { // what is above the first unit's doc is the header
			u.Pos = b.lineStart(u.Doc)
		} else {
			u.Pos = b.nextLine(b.us[i-1].To, u.From)
		}
	}
	if len(b.us) == 0 || b.us[0].Pos > 0 {
		end := len(p.rs)
		if len(b.us) > 0 {
			end = b.us[0].Pos
		}
		p.Units = append(p.Units, Unit{Kind: Header, Pos: 0, End: end, From: 0, To: end})
	}
	for i, u := range b.us {
		u.End = len(p.rs)
		if i+1 < len(b.us) {
			u.End = b.us[i+1].Pos
		}
		p.Units = append(p.Units, u)
	}
	return p
}

// Span is a unit's whole text; Code its code alone.
func (p *Program) Span(i int) string { return string(p.rs[p.Units[i].Pos:p.Units[i].End]) }
func (p *Program) Code(i int) string { return string(p.rs[p.Units[i].From:p.Units[i].To]) }

// Replace gives the program with unit i's whole span replaced by text, read again.
func (p *Program) Replace(i int, text string) *Program {
	u := p.Units[i]
	return Parse(string(p.rs[:u.Pos]) + text + string(p.rs[u.End:]))
}

// At is the unit offset off (in runes) is in.
func (p *Program) At(off int) int {
	for i, u := range p.Units {
		if off < u.End {
			return i
		}
	}
	return len(p.Units) - 1
}

// Outline is a unit a line: kind, name, its lines, and its code's first line.
func (p *Program) Outline() string {
	var b strings.Builder
	for i, u := range p.Units {
		first := strings.TrimSpace(strings.SplitN(p.Code(i), "\n", 2)[0])
		if u.Kind == Header {
			first = strings.TrimSpace(strings.SplitN(p.Span(i), "\n", 2)[0])
		}
		if r := []rune(first); len(r) > 50 {
			first = string(r[:49]) + "…"
		}
		fmt.Fprintf(&b, "%-6s %-20s %-7s %s\n", u.Kind, u.Name, p.Lines(i), first)
	}
	return b.String()
}

// Lines are the lines unit i runs over, from its span's start to its code's end: 24-25, or
// 27 when it is one.
func (p *Program) Lines(i int) string {
	u := p.Units[i]
	a, b := p.line(u.Pos), p.line(max(u.To-1, u.Pos))
	if a == b {
		return fmt.Sprint(a)
	}
	return fmt.Sprintf("%d-%d", a, b)
}

// line is the line offset off is on, from 1.
func (p *Program) line(off int) int {
	return 1 + strings.Count(string(p.rs[:min(off, len(p.rs))]), "\n")
}

type builder struct {
	rs []rune
	ts []jqsyntax.Token
	us []Unit
}

func (b *builder) add(k Kind, name string, from, to int, n *jqsyntax.Node) {
	b.us = append(b.us, Unit{Kind: k, Name: name, From: from, To: to, Node: n})
}

func (b *builder) text(from, to int) string { return string(b.rs[from:to]) }

// top is the whole tree: directives, then the top-level defs, then main.
func (b *builder) top(n *jqsyntax.Node) {
	switch n.Kind {
	case jqsyntax.NModule:
		for _, k := range n.Kids {
			if k.Kind == jqsyntax.NDirective {
				b.add(Directive, k.Text, k.Pos, k.End, k)
			} else {
				b.top(k)
			}
		}
	case jqsyntax.NDefs:
		for _, k := range n.Kids {
			if k.Kind == jqsyntax.NDef {
				b.add(Def, b.signature(k), k.Pos, k.End, k)
			} else {
				b.main(k, -1)
			}
		}
	default:
		b.main(n, -1)
	}
}

// main walks main's spine: a | b, src as $x | body, def …; query. sep is where the | before
// n is, or -1.
func (b *builder) main(n *jqsyntax.Node, sep int) {
	from := n.Pos
	if sep >= 0 {
		from = sep
	}
	k := n.Kids
	switch {
	case n.Kind == jqsyntax.NMissing:
	case n.Kind == jqsyntax.NPipe:
		b.add(Stage, "", from, k[0].End, k[0])
		b.main(k[1], b.pipe(k[0].End, k[1].Pos))
	case n.Kind == jqsyntax.NBind && !n.Open:
		pats := k[1 : len(k)-1]
		to := pats[len(pats)-1].End
		var names []string
		for _, p := range pats {
			names = append(names, b.text(p.Pos, p.End))
		}
		b.add(Stage, "as "+strings.Join(names, " ?// "), from, to, n)
		b.main(k[len(k)-1], b.pipe(to, k[len(k)-1].Pos))
	case n.Kind == jqsyntax.NDefs:
		for i, d := range k {
			switch {
			case d.Kind != jqsyntax.NDef:
				b.main(d, -1)
			case i == 0: // only the first takes the | before it
				b.add(Stage, "def "+b.signature(d), from, d.End, d)
			default:
				b.add(Stage, "def "+b.signature(d), d.Pos, d.End, d)
			}
		}
	default:
		b.add(Stage, "", from, n.End, n)
	}
}

// signature is a def's name and parameters as written: members($depth).
func (b *builder) signature(d *jqsyntax.Node) string {
	var ps []string
	for _, k := range d.Kids {
		if k.Kind == jqsyntax.NParam {
			ps = append(ps, k.Text)
		}
	}
	if len(ps) == 0 {
		return d.Text
	}
	return d.Text + "(" + strings.Join(ps, "; ") + ")"
}

// pipe is where the | between from and to is, or -1.
func (b *builder) pipe(from, to int) int {
	for _, t := range b.ts {
		if t.Kind == jqsyntax.Pipe && t.Pos >= from && t.Pos < to {
			return t.Pos
		}
	}
	return -1
}

// docStart is where the doc of the code at from starts: the comment lines just above it, no
// blank line between, each starting its line; then back to that line's start.
func (b *builder) docStart(from int) int {
	j := 0
	for j < len(b.ts) && b.ts[j].Pos < from {
		j++
	}
	start := from
	for k := j - 1; k >= 1; k -= 2 {
		sp, c := b.ts[k], b.ts[k-1]
		if sp.Kind != jqsyntax.Space || strings.Count(sp.Text, "\n") != 1 || c.Kind != jqsyntax.Comment {
			break
		}
		if k-2 >= 0 && !(b.ts[k-2].Kind == jqsyntax.Space && (strings.Contains(b.ts[k-2].Text, "\n") || k-2 == 0)) {
			break // a comment after code on its line is that code's, not a doc
		}
		start = c.Pos
	}
	return start
}

// lineStart is the start of off's line when only spaces come before it there, else off.
func (b *builder) lineStart(off int) int {
	i := off
	for i > 0 && (b.rs[i-1] == ' ' || b.rs[i-1] == '\t') {
		i--
	}
	if i == 0 || b.rs[i-1] == '\n' {
		return i
	}
	return off
}

// nextLine is where a unit starts whose code is at from, after a unit whose code ends at to:
// the first line after to's that isn't blank, or from itself when it is on to's line.
func (b *builder) nextLine(to, from int) int {
	i := to
	for i < from && b.rs[i] != '\n' {
		i++
	}
	if i == from {
		return from
	}
	for i < from { // i is on a \n: skip lines with nothing on them
		j := i + 1
		for j < from && (b.rs[j] == ' ' || b.rs[j] == '\t' || b.rs[j] == '\r') {
			j++
		}
		if j < len(b.rs) && b.rs[j] == '\n' && j < from {
			i = j
			continue
		}
		return min(i+1, b.lineStart(from))
	}
	return b.lineStart(from)
}
