package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/program"
	"github.com/TimelordUK/jqv/internal/query"
	"github.com/itchyny/gojq"
)

// loaded is a program read in by :load (Q-032, DESIGN §23). Its text goes in front of every
// line, so its defs are in reach, and so are main's variables and the defs written in main
// (a line goes after main with a |, inside every `as`'s body); . is what main gives.
type loaded struct {
	path string
	prog *program.Program
	main int // the first of main's stages, or -1 when the file is only defs
}

// name is how the prompt names the program: its file and the folder it is in, since an
// example is always query.jq.
func (l *loaded) name() string {
	return filepath.Base(filepath.Dir(l.path)) + "/" + filepath.Base(l.path)
}

// defs is the text in front of main: the header, directives and top-level defs.
func (l *loaded) defs() string {
	if l.main < 0 {
		return l.prog.Text
	}
	return string([]rune(l.prog.Text)[:l.prog.Units[l.main].Pos])
}

// around puts the program around q, the stages and the line: after main with a |, or after
// the defs when there is no main.
func (l *loaded) around(q string) string {
	switch {
	case l.main >= 0 && q == "":
		return l.prog.Text
	case l.main >= 0:
		return l.prog.Text + "\n| " + q
	case q == "":
		return l.prog.Text + "\n."
	}
	return l.prog.Text + "\n" + q
}

// directives says whether the program starts with module, import or include, which nothing
// may come before.
func (l *loaded) directives() bool {
	for _, u := range l.prog.Units {
		if u.Kind != program.Header {
			return u.Kind == program.Directive
		}
	}
	return false
}

// vars are the variables every run has: $root, $_, and the program's arguments, which win.
func (s *Session) vars() map[string]any {
	v := map[string]any{"$root": s.root, "$_": s.lastVal}
	for k, a := range s.args {
		v["$"+k] = a
	}
	return v
}

// onDocument says whether . is the whole document: no stages, and no main loaded.
func (s *Session) onDocument() bool {
	return len(s.stages) == 0 && (s.prog == nil || s.prog.main < 0)
}

// load is :load file.jq: the program in front of every line from now on. A file that doesn't
// parse isn't loaded. The stages are dropped, since . is now what main gives, and a case.json
// beside the file (an example's) gives the arguments.
func (s *Session) load(path string) string {
	if path == "" {
		if s.prog == nil {
			return "usage: :load file.jq\n"
		}
		return "loaded: " + s.prog.path + "  " + s.counts() + "\n"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "not loaded: " + err.Error() + "\n"
	}
	text := string(b)
	if _, err := gojq.Parse(text); err != nil {
		return "not loaded, it doesn't parse:\n" + failure(text, err)
	}
	l := &loaded{path: filepath.ToSlash(path), prog: program.Parse(text), main: -1}
	for i, u := range l.prog.Units {
		if u.Kind == program.Stage {
			l.main = i
			break
		}
	}
	var out strings.Builder
	if n := len(s.stages); n > 0 {
		fmt.Fprintf(&out, "dropped %d stage%s: . is now what the program gives\n", n, plural(n))
	}
	s.prog, s.stages, s.last, s.shapes = l, nil, "", nil
	if c := caseArgs(filepath.Join(filepath.Dir(path), "case.json")); len(c) > 0 {
		if s.args == nil {
			s.args = map[string]string{}
		}
		for k, v := range c {
			s.args[k] = v
		}
		out.WriteString("arguments from case.json: " + s.argList() + "\n")
	}
	fmt.Fprintf(&out, "loaded %s: %s", l.name(), s.counts())
	if l.main >= 0 {
		out.WriteString("  → " + summary(s.run("")))
	}
	out.WriteString("\n:defs lists them with what each gives\n")
	return out.String()
}

// caseArgs are an example's --arg values, from its case.json; nil without one.
func caseArgs(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c struct {
		Args map[string]string `json:"args"`
	}
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	return c.Args
}

// counts says what the loaded program has: 2 defs, main in 6 stages.
func (s *Session) counts() string {
	defs, stages := 0, 0
	for _, u := range s.prog.prog.Units {
		switch u.Kind {
		case program.Def:
			defs++
		case program.Stage:
			stages++
		}
	}
	main := "no main"
	if stages > 0 {
		main = fmt.Sprintf("main in %d stage%s", stages, plural(stages))
	}
	return fmt.Sprintf("%d def%s, %s", defs, plural(defs), main)
}

var oneVar = regexp.MustCompile(`^as (\$[A-Za-z_][A-Za-z0-9_]*)$`)

// defsList is :defs: the loaded program's units, a line each, with what each gives. A def
// without arguments runs on the document; main's stages give what main gives up to them,
// and an `as` the value it names. A def that takes arguments waits for taps (DESIGN §23).
func (s *Session) defsList() string {
	if s.prog == nil {
		return "no program loaded: :load file.jq\n"
	}
	p := s.prog.prog
	var b strings.Builder
	for i, u := range p.Units {
		name, gives := u.Name, ""
		switch u.Kind {
		case program.Header:
			continue
		case program.Directive:
			gives = strings.TrimSpace(p.Code(i))
		case program.Def:
			if u.Name != u.Node.Text { // it has parameters
				gives = "takes arguments: what it gives waits for taps"
			} else {
				gives = "→ " + gist(s.runText(s.prog.defs()+"\n"+u.Name)) + ", on the document"
			}
		case program.Stage:
			upto := string([]rune(p.Text)[:u.To])
			switch m := oneVar.FindStringSubmatch(u.Name); {
			case m != nil:
				gives = m[1] + " = " + gist(s.runText(upto+"\n| "+m[1]))
			case strings.HasPrefix(u.Name, "as "):
				gives = "→ " + gist(s.runText(upto+"\n| ."))
			case strings.HasPrefix(u.Name, "def ") && u.Name != "def "+u.Node.Text:
				gives = "takes arguments: what it gives waits for taps"
			case strings.HasPrefix(u.Name, "def "):
				gives = "→ " + gist(s.runText(upto+"\n"+u.Node.Text))
			default:
				name = firstLine(p.Code(i))
				gives = "→ " + gist(s.runText(upto))
			}
		}
		fmt.Fprintf(&b, "%-6s %-20s %-7s %s\n", u.Kind, name, p.Lines(i), gives)
	}
	return b.String()
}

// firstLine is a stage's code as a name: its first line without the | before it or a comment
// after it, cut short.
func firstLine(code string) string {
	var b strings.Builder
	for _, t := range jqsyntax.Lex(code) {
		if t.Kind == jqsyntax.Comment {
			continue
		}
		if t.Kind == jqsyntax.Space && strings.Contains(t.Text, "\n") && strings.Trim(b.String(), "| ") != "" {
			break
		}
		b.WriteString(t.Text)
	}
	l := strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(b.String()), "|")), " ")
	if r := []rune(l); len(r) > 20 {
		l = string(r[:19]) + "…"
	}
	return l
}

// gist is what a run gave, as summary says it, but a single number, string, boolean or null
// shown as itself when short: $min = 30.
func gist(r query.Result) string {
	if r.Err == nil && !r.More && len(r.Values) == 1 {
		switch r.Values[0].(type) {
		case []any, map[string]any:
		default:
			if j, err := gojq.Marshal(r.Values[0]); err == nil && len(j) <= 40 {
				return string(j)
			}
		}
	}
	return summary(r)
}

// runText runs a whole program, as written, on the document.
func (s *Session) runText(text string) query.Result {
	return query.Run(text, s.root, s.vars(), s.Opt)
}

// arg is :arg: the program's --arg values listed, one set (:arg min 30), or dropped (:arg
// min). Like --arg, a value is a string.
func (s *Session) arg(a string) string {
	if a == "" {
		if len(s.args) == 0 {
			return "no arguments: :arg name value gives the program $name, as jq's --arg does\n"
		}
		return s.argList() + "\n"
	}
	name, val, ok := strings.Cut(a, " ")
	name = strings.TrimPrefix(name, "$")
	s.shapes = nil
	if !ok {
		if _, had := s.args[name]; !had {
			return "no argument $" + name + "\n"
		}
		delete(s.args, name)
		return "dropped $" + name + "\n"
	}
	if s.args == nil {
		s.args = map[string]string{}
	}
	s.args[name] = strings.TrimSpace(val)
	out := fmt.Sprintf("$%s = %q\n", name, s.args[name])
	if name == "root" {
		out += "$root is now this argument, not the document\n"
	}
	return out
}

// argList is the arguments as `$min = "30", $type = "p"`.
func (s *Session) argList() string {
	names := make([]string, 0, len(s.args))
	for k := range s.args {
		names = append(names, k)
	}
	sort.Strings(names)
	for i, k := range names {
		names[i] = fmt.Sprintf("$%s = %q", k, s.args[k])
	}
	return strings.Join(names, ", ")
}
