// Package bench is the scratchpad REPL's engine (DESIGN §18, Q-020): lines of jq run against
// the current `.` and forgotten, until one is pushed as a stage. The stages joined with `|`
// are one jq program, and a line runs as `stages | line` on the whole document, so it means
// exactly what it would in jq.
//
// A Session takes lines and gives text, with no terminal, so it is tested with transcripts
// (testdata/*.session against *.golden) and can sit under the CLI and, later, the TUI's reader
// (Q-017).
package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TimelordUK/jqv/internal/catalog"
	"github.com/TimelordUK/jqv/internal/query"
	"github.com/itchyny/gojq"
)

// Session is one REPL: the document, the stages pushed so far, and the last line's result.
type Session struct {
	Opt   query.Options
	Width int    // a value printed on one line up to this width, pretty beyond it
	Pipe  string // what a bare :! runs: config.toml's [repl] pipe, e.g. "bat -l json"

	root    any
	stages  []string
	last    string // the last line that ran without an error, for :push and :tree
	lastOut []any  // its outputs, for :pp and :!
	lastVal any    // $_

	examples []catalog.Example // the last :help's, for :try N
}

// Reply is what a line gives: text to show, and anything the front end has to do that a
// session can't (it has no terminal, and runs no processes).
type Reply struct {
	Text  string
	Quit  bool
	Clear bool   // clear the screen before showing Text
	Pipe  string // run this command with Input on its stdin
	Input string
	Emit  string // with Quit: the program worked out, for the front end to hand to the shell

	Failed bool // a line of jq that didn't parse or run: history keeps it, marked (R-001)
}

// writeQuit is :wq [expr]: quit, handing back the program, which is the stages and then expr,
// or else the last line run since the last :push (what :push would add).
func (s *Session) writeQuit(expr string) Reply {
	if expr == "" {
		expr = s.last
	}
	if usesLast.MatchString(expr) {
		return Reply{Text: "not quitting: the line uses $_, which only the REPL has. Use $root, or a value\n"}
	}
	if expr != "" {
		if _, err := gojq.Parse(expr); err != nil {
			return Reply{Text: "not quitting:\n" + failure(expr, err)}
		}
	}
	return Reply{Quit: true, Emit: s.Program(expr)}
}

// New starts a session on a document's value (doc.Node.Value()).
func New(root any) *Session {
	return &Session{
		Opt:   query.Options{Timeout: 5 * time.Second, Max: 50},
		Width: 100,
		root:  root,
	}
}

// Stages are the stages pushed so far.
func (s *Session) Stages() []string { return append([]string(nil), s.stages...) }

// Prompt shows where `.` is: the stages, shortened from the left when long.
func (s *Session) Prompt() string {
	p := "."
	if len(s.stages) > 0 {
		p = strings.Join(s.stages, " | ")
		if r := []rune(p); len(r) > 40 {
			p = "…" + string(r[len(r)-39:])
		}
	}
	return p + " ❯ "
}

var (
	usesLast = regexp.MustCompile(`\$_\b`)
	usesRoot = regexp.MustCompile(`\$root\b`)
)

// Program is the stages as one jq program, then extra if given. It starts with
// `. as $root |` when anything uses $root, so it runs in plain jq as it does here.
func (s *Session) Program(extra string) string {
	parts := append(s.Stages(), extra)
	if extra == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return "."
	}
	p := strings.Join(parts, " | ")
	if usesRoot.MatchString(p) {
		p = ". as $root | " + p
	}
	return p
}

// run gives what the stages and then expr give on the document.
func (s *Session) run(expr string) query.Result {
	vars := map[string]any{"$root": s.root, "$_": s.lastVal}
	return query.Run(s.Program(expr), s.root, vars, s.Opt)
}

// Do runs one line, jq or a :command.
func (s *Session) Do(line string) Reply {
	line = strings.TrimSpace(line)
	if line == "" {
		return Reply{}
	}
	if strings.HasPrefix(line, ":") {
		return s.command(line)
	}
	text, ok := s.eval(line)
	return Reply{Text: text, Failed: !ok}
}

// eval runs a line of jq and gives what to show, and whether it ran without an error.
func (s *Session) eval(line string) (string, bool) {
	if _, err := gojq.Parse(line); err != nil { // parse the line alone, so the caret is on it
		return failure(line, err), false
	}
	r := s.run(line)
	var b strings.Builder
	for _, v := range r.Values {
		b.WriteString(s.format(v))
		b.WriteByte('\n')
	}
	if r.Err != nil {
		b.WriteString(failure(line, r.Err))
		return b.String(), false
	}
	switch n := len(r.Values); {
	case r.More:
		fmt.Fprintf(&b, "(the first %d outputs; there are more)\n", n)
	case n == 0:
		b.WriteString("(no output)\n")
	case n > 1:
		fmt.Fprintf(&b, "(%d outputs)\n", n)
	}
	if allNull(r.Values) {
		b.WriteString(s.nullNote())
		if h := funcAsField(line); h != "" {
			b.WriteString("hint: " + h + "\n")
		}
	}
	s.remember(line, r)
	return b.String(), true
}

// remember makes line the last line, for :push, :tree, :pp and $_.
func (s *Session) remember(line string, r query.Result) {
	s.last = line
	s.lastOut = r.Values
	if len(r.Values) == 1 && !r.More {
		s.lastVal = r.Values[0]
	} else {
		s.lastVal = append([]any{}, r.Values...)
	}
}

func allNull(vs []any) bool {
	for _, v := range vs {
		if v != nil {
			return false
		}
	}
	return len(vs) > 0
}

// nullNote is said when a line gives only nulls, which is usually a misspelt field: jq gives
// null for a field that isn't there. It lists the keys of what . is, when that is an object.
func (s *Session) nullNote() string {
	note := "(all null: a field that isn't there is null"
	if r := s.run(""); r.Err == nil && len(r.Values) > 0 {
		if m, ok := r.Values[0].(map[string]any); ok && len(m) > 0 {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			note += ". Keys here: " + strings.Join(keys, ", ")
		}
	}
	return note + ")\n"
}

func failure(expr string, err error) string {
	var b strings.Builder
	if c := query.Caret(expr, err); c != "" {
		b.WriteString(c + "\n")
	}
	b.WriteString("error: " + err.Error() + "\n")
	if h := query.ParseHint(expr, err); h != "" {
		b.WriteString("hint: " + h + "\n")
	} else if h := funcAsField(expr); h != "" {
		b.WriteString("hint: " + h + "\n")
	} else if h := query.Hint(err); h != "" {
		b.WriteString("hint: " + h + "\n")
	}
	return b.String()
}

var fieldName = regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)\b`)

// funcAsField catches `.name.length`: a function written as if it were a field, as in
// languages with methods. In jq `.length` asks for a field called length.
func funcAsField(expr string) string {
	for _, m := range fieldName.FindAllStringSubmatch(expr, -1) {
		if _, ok := funcs[m[1]]; ok {
			return fmt.Sprintf(".%s asks for a field called %q; %s is a function, so pipe into it: … | %s",
				m[1], m[1], m[1], m[1])
		}
	}
	return ""
}

const help = `A line of jq runs on the current . and is forgotten. :push keeps it as a stage.
  :push [expr]  :p   make the last line (or expr) a stage: . is now what it gives
  :pop               take the last stage back
  :reset             drop every stage
  :stages            the stages, with what each gives
  :program           the stages as one jq program, to copy
  :tree [expr]       the last line (or expr) step by step, with what each step gives
  :pp [expr]         the last result (or expr's) pretty-printed
  :! cmd args        the last result, pretty, into a program: :! bat -l json
  :!                 the same into [repl] pipe from config.toml (pipe = "bat -l json")
  :cls               clear the screen (:clear too); the session and history stay
  :help              this
  :help name         jq's manual on a function, operator or section: :help trim, :help |
  :help jq           everything the manual covers
  :help functions    every function's name
  :try N [.]         run example N from the last :help on its input, or with . on yours
  :wq [expr]         quit, and print the program (stages, then the last line) as a jq command
  :q                 quit (or ctrl+z, enter on Windows; ctrl+d elsewhere)
$root is the whole document; $_ is the last line's result (an array if it gave several).`

func (s *Session) command(line string) Reply {
	name, arg, _ := strings.Cut(line[1:], " ")
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(name, "!") { // :!bat as well as :! bat
		arg = strings.TrimSpace(name[1:] + " " + arg)
		name = "!"
	}
	text := func(t string) Reply { return Reply{Text: t} }
	switch name {
	case "q", "quit", "exit":
		return Reply{Quit: true}
	case "wq", "x":
		return s.writeQuit(arg)
	case "help", "h", "?":
		if arg != "" {
			return text(s.manual(arg))
		}
		return text(help + "\n")
	case "try":
		return text(s.try(arg))
	case "cls", "clear":
		return Reply{Clear: true}
	case "p", "push":
		return text(s.push(arg))
	case "pop":
		if len(s.stages) == 0 {
			return text("no stages to pop\n")
		}
		e := s.stages[len(s.stages)-1]
		s.stages = s.stages[:len(s.stages)-1]
		return text("popped: " + e + "\n")
	case "reset":
		n := len(s.stages)
		s.stages = nil
		return text(fmt.Sprintf("dropped %d stage%s\n", n, plural(n)))
	case "stages":
		if len(s.stages) == 0 {
			return text("no stages: . is the whole document\n")
		}
		var b strings.Builder
		all := s.stages
		for i := range all {
			s.stages = all[:i+1]
			fmt.Fprintf(&b, "%d  %s  → %s\n", i+1, all[i], summary(s.run("")))
		}
		s.stages = all
		return text(b.String())
	case "program", "prog":
		return text(s.Program("") + "\n")
	case "tree", "t":
		return text(s.tree(arg))
	case "pp":
		vs, msg := s.values(arg)
		if msg != "" {
			return text(msg)
		}
		return text(pretty(vs))
	case "!":
		if arg == "" {
			arg = s.Pipe
		}
		if arg == "" {
			return text("usage: :! cmd args, e.g. :! bat -l json; or set pipe = \"bat -l json\" under [repl] in config.toml\n")
		}
		vs, msg := s.values("")
		if msg != "" {
			return text(msg)
		}
		return Reply{Pipe: arg, Input: pretty(vs)}
	}
	return text("unknown command :" + name + " (:help lists them)\n")
}

// values are expr's outputs, or the last line's when expr is empty. msg says why there are
// none to give.
func (s *Session) values(expr string) (vs []any, msg string) {
	if expr == "" {
		if s.last == "" {
			return nil, "nothing yet: run a line first\n"
		}
		return s.lastOut, ""
	}
	if _, err := gojq.Parse(expr); err != nil {
		return nil, failure(expr, err)
	}
	r := s.run(expr)
	if r.Err != nil {
		return nil, failure(expr, r.Err)
	}
	return r.Values, ""
}

// pretty writes values as jq does by default: indented, one after another.
func pretty(vs []any) string {
	var b strings.Builder
	for _, v := range vs {
		j, err := gojq.Marshal(v)
		if err != nil {
			fmt.Fprintln(&b, v)
			continue
		}
		var out bytes.Buffer
		if json.Indent(&out, j, "", "  ") != nil {
			out.Reset()
			out.Write(j)
		}
		b.Write(out.Bytes())
		b.WriteByte('\n')
	}
	return b.String()
}

func (s *Session) push(expr string) string {
	if expr == "" {
		expr = s.last
	}
	if expr == "" {
		return "nothing to push: run a line first, or :push expr\n"
	}
	if usesLast.MatchString(expr) {
		return "a stage can't use $_: it changes with every line. Use $root, or a value\n"
	}
	if _, err := gojq.Parse(expr); err != nil {
		return failure(expr, err)
	}
	if r := s.run(expr); r.Err != nil {
		return "not pushed, it fails here:\n" + failure(expr, r.Err)
	}
	s.stages = append(s.stages, expr)
	s.last = ""
	return fmt.Sprintf("stage %d: %s  → %s\n", len(s.stages), expr, summary(s.run("")))
}

// summary says what a run gave, in a few words: "array of 2", "3 outputs".
func summary(r query.Result) string {
	switch {
	case r.Err != nil:
		return "error: " + r.Err.Error()
	case r.More:
		return fmt.Sprintf("more than %d outputs", len(r.Values))
	case len(r.Values) == 0:
		return "no output"
	case len(r.Values) == 1:
		return kind(r.Values[0])
	}
	n := fmt.Sprintf("%d outputs", len(r.Values))
	t := typeName(r.Values[0])
	for _, v := range r.Values[1:] {
		if typeName(v) != t {
			return n
		}
	}
	return n + " (" + t + "s)"
}

// typeName is jq's name for a value's type.
func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "number"
}

func kind(v any) string {
	switch v := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case int, float64, *big.Int, json.Number:
		return "number"
	case []any:
		return "array of " + strconv.Itoa(len(v))
	case map[string]any:
		return fmt.Sprintf("object, %d key%s", len(v), plural(len(v)))
	}
	return fmt.Sprintf("%T", v)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// format writes a value on one line when it fits in Width, pretty-printed otherwise.
func (s *Session) format(v any) string {
	b, err := gojq.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if len(b) <= s.Width {
		return string(b)
	}
	var out bytes.Buffer
	if json.Indent(&out, b, "", "  ") != nil {
		return string(b)
	}
	return out.String()
}

// Transcript runs lines through the session as a terminal would show them: the prompt and
// the line typed, then what it gave. It stops at a :q. What the front end would do is noted
// in brackets: [screen cleared], [into bat -l json] and the text that would go in.
func Transcript(s *Session, lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(strings.TrimRight(s.Prompt()+l, " ") + "\n")
		r := s.Do(l)
		if r.Clear {
			b.WriteString("[screen cleared]\n")
		}
		b.WriteString(r.Text)
		if r.Pipe != "" {
			b.WriteString("[into " + r.Pipe + "]\n" + r.Input)
		}
		if r.Emit != "" {
			b.WriteString("[emit] " + r.Emit + "\n")
		}
		if r.Quit {
			break
		}
	}
	return b.String()
}
