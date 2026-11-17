package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/program"
	"github.com/TimelordUK/jqv/internal/query"
	"github.com/itchyny/gojq"
)

// editing is a unit of the loaded program out in the editor (:e, DESIGN §23 step 7): the
// next line puts it back over from to to (runes in the program's text).
type editing struct {
	label    string
	from, to int
}

// Editing is the name of what is out in the editor, or "".
func (s *Session) Editing() string {
	if s.edit == nil {
		return ""
	}
	return s.edit.label
}

// Unsaved says whether the loaded program has edits :w hasn't written.
func (s *Session) Unsaved() bool { return s.prog != nil && s.prog.changed }

func (s *Session) unsaved(drop string) string {
	return fmt.Sprintf("unsaved changes to %s: :w saves them, or %s drops them\n", s.prog.name(), drop)
}

// Abandon is ctrl+c while a unit is out in the editor: the program stays as it was.
func (s *Session) Abandon() string {
	if s.edit == nil {
		return ""
	}
	l := s.edit.label
	s.edit = nil
	return "not changed: " + l + "\n"
}

var varName = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// unitNames are what :e knows a unit by: a def by its name (and with its parameters), a
// stage that binds by its variables, a def written in main by its name. Main's other stages
// are reached through `main`.
func unitNames(u program.Unit) []string {
	switch {
	case u.Kind == program.Header:
		return []string{"header"}
	case u.Kind == program.Def:
		return []string{u.Node.Text, u.Name}
	case u.Kind == program.Stage && strings.HasPrefix(u.Name, "def "):
		return []string{u.Node.Text, u.Name[4:]}
	case u.Kind == program.Stage && strings.HasPrefix(u.Name, "as "):
		return varName.FindAllString(u.Name, -1)
	}
	return nil
}

// EditName is a name :e takes, with a few words on what it is, for Tab.
type EditName struct{ Name, About string }

// EditNames are the names :e takes in the loaded program, in its order: each def and named
// stage, then main.
func (s *Session) EditNames() []EditName {
	if s.prog == nil {
		return nil
	}
	p := s.prog.prog
	var ns []EditName
	for i, u := range p.Units {
		if names := unitNames(u); len(names) > 0 {
			about := fmt.Sprintf("%s %s, lines %s", u.Kind, u.Name, p.Lines(i))
			if u.Kind == program.Header {
				about = "the comment at the top, lines " + p.Lines(i)
			}
			ns = append(ns, EditName{names[0], about})
		}
	}
	if s.prog.main >= 0 {
		ns = append(ns, EditName{"main", fmt.Sprintf("the pipeline at the bottom, %d stages", len(p.Units)-s.prog.main)})
	}
	return ns
}

// find is the part of the program :e name puts in the editor.
func (s *Session) find(name string) (from, to int, ok bool) {
	p := s.prog.prog
	if name == "main" && s.prog.main >= 0 {
		from, _ = p.Editable(s.prog.main)
		_, to = p.Editable(len(p.Units) - 1)
		return from, to, true
	}
	found := -1
	for i, u := range p.Units {
		for _, n := range unitNames(u) {
			if n == name {
				found = i // the last: a def given twice, it is the last that main calls
			}
		}
	}
	if found < 0 {
		return 0, 0, false
	}
	from, to = p.Editable(found)
	return from, to, true
}

// editUnit is :e name: that def or stage put in the editor, its doc comment with it. Enter
// puts it back; :e alone is the last one edited again.
func (s *Session) editUnit(name string) Reply {
	if s.prog == nil {
		return Reply{Text: "no program loaded: :load file.jq, then :e name\n"}
	}
	if name == "" {
		name = s.lastEdit
	}
	if name == "" {
		var ns []string
		for _, n := range s.EditNames() {
			ns = append(ns, n.Name)
		}
		return Reply{Text: "usage: :e name, one of: " + strings.Join(ns, " ") + "\n"}
	}
	from, to, ok := s.find(name)
	if !ok {
		return Reply{Text: fmt.Sprintf("no def or stage %s in %s: Tab after :e offers them, :defs lists them\n", name, s.prog.name())}
	}
	s.edit = &editing{label: name, from: from, to: to}
	text := strings.ReplaceAll(string([]rune(s.prog.prog.Text)[from:to]), "\r\n", "\n")
	return Reply{Edit: text}
}

// putBack is the line after :e: what was edited goes back in its place, if the program still
// parses with it; else it goes back to the editor with the error. An empty line, or a
// command, leaves the program as it was.
func (s *Session) putBack(line string) Reply {
	e := s.edit
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, ":") {
		s.edit = nil
		r := Reply{}
		if t != "" {
			r = s.command(t)
		}
		r.Text = "not changed: " + e.label + "\n" + r.Text
		return r
	}
	text := s.prog.lineEnds(strings.TrimRight(line, " \t\r\n"))
	rs := []rune(s.prog.prog.Text)
	whole := string(rs[:e.from]) + text + string(rs[e.to:])
	if _, err := gojq.Parse(whole); err != nil {
		return Reply{Text: "not put back, the program doesn't parse with it:\n" + wholeFailure(whole, err) +
			"still editing " + e.label + ": put it right, or enter on an empty line leaves it as it was\n",
			Edit: line, Failed: true}
	}
	s.edit, s.lastEdit = nil, e.label
	s.reload(whole)
	return Reply{Text: "put back: " + e.label + "\n" + s.linesIn(e.from, e.from+len([]rune(text))) + s.mainGives()}
}

// wholeFailure is why a whole program doesn't parse: the caret on the error's line, and jq's
// error. A line's guesses (a field that is a function's name) would be about the whole file.
func wholeFailure(text string, err error) string {
	var b strings.Builder
	if c := query.Caret(text, err); c != "" {
		b.WriteString(c + "\n")
	}
	b.WriteString("error: " + err.Error() + "\n")
	if h := query.ParseHint(text, err); h != "" {
		b.WriteString("hint: " + h + "\n")
	}
	return b.String()
}

// lineEnds gives text the file's line ends.
func (l *loaded) lineEnds(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if l.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// reload makes text the loaded program, edited.
func (s *Session) reload(text string) {
	l := read(s.prog.path, text)
	l.changed = true
	s.prog, s.shapes = l, nil
}

// linesIn are the :defs lines of the units whose code is in from to to.
func (s *Session) linesIn(from, to int) string {
	var b strings.Builder
	for i, u := range s.prog.prog.Units {
		if u.From < to && u.To > from {
			b.WriteString(s.unitLine(i))
		}
	}
	return b.String()
}

// mainGives says what main gives now, when there is one.
func (s *Session) mainGives() string {
	if s.prog.main < 0 {
		return ""
	}
	return "main → " + summary(s.runText(s.prog.prog.Text)) + "\n"
}

// defsOnly says whether a line is only defs, which jq won't run alone ("Top-level program
// not given") and so can only mean: keep these.
func defsOnly(line string) bool {
	n := jqsyntax.Parse(line)
	return n.Whole() && n.Kind == jqsyntax.NDefs && n.Kids[len(n.Kids)-1].Kind == jqsyntax.NDef
}

// addDefs puts a line of defs into the loaded program (DESIGN §20): a def with the name and
// arity of a top-level one takes its place, keeping its doc comment, since the defs after it
// call it there; a new one goes after the others, before main.
func (s *Session) addDefs(line string) string {
	rs := []rune(line)
	var b strings.Builder
	for _, d := range jqsyntax.Parse(line).Kids {
		text := s.prog.lineEnds(string(rs[d.Pos:d.End]))
		p := s.prog.prog
		i := s.topDef(d)
		var whole, did string
		switch {
		case i >= 0:
			u := p.Units[i]
			whole, did = string([]rune(p.Text)[:u.From])+text+string([]rune(p.Text)[u.To:]), "replaced"
		case s.prog.main >= 0:
			at := p.Units[s.prog.main].Pos
			whole, did = string([]rune(p.Text)[:at])+text+s.prog.lineEnds("\n\n")+string([]rune(p.Text)[at:]), "added"
		default:
			before := strings.TrimRight(p.Text, " \t\r\n")
			if before != "" {
				before += s.prog.lineEnds("\n\n")
			}
			whole, did = before+text+s.prog.lineEnds("\n"), "added"
		}
		if _, err := gojq.Parse(whole); err != nil { // a def can't break the program, but say so if it does
			b.WriteString("not added:\n" + wholeFailure(whole, err))
			break
		}
		s.reload(whole)
		fmt.Fprintf(&b, "%s: %s\n", did, s.prog.prog.Units[s.topDef(d)].Name)
		b.WriteString(s.unitLine(s.topDef(d)))
	}
	return b.String() + s.mainGives()
}

// topDef is the top-level def with d's name and arity (the last, if it is given twice), or -1.
func (s *Session) topDef(d *jqsyntax.Node) int {
	arity := func(n *jqsyntax.Node) int {
		c := 0
		for _, k := range n.Kids {
			if k.Kind == jqsyntax.NParam {
				c++
			}
		}
		return c
	}
	found := -1
	for i, u := range s.prog.prog.Units {
		if u.Kind == program.Def && u.Node.Text == d.Text && arity(u.Node) == arity(d) {
			found = i
		}
	}
	return found
}

// write is :w [file]: the loaded program written to its file, or to file. Over another file
// that is there already only as :w!.
func (s *Session) write(path string, force bool) string {
	if s.prog == nil {
		return "no program loaded: :load file.jq\n"
	}
	own := path == "" || filepath.Clean(path) == filepath.Clean(s.prog.path)
	if own {
		path = s.prog.path
	} else if _, err := os.Stat(path); err == nil && !force {
		return path + " is there already: :w! " + path + " writes over it\n"
	}
	text := s.prog.prog.Text
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "not written: " + err.Error() + "\n"
	}
	if own {
		s.prog.changed = false
	}
	n := strings.Count(text, "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		n++
	}
	return fmt.Sprintf("wrote %s: %d line%s\n", path, n, plural(n))
}
