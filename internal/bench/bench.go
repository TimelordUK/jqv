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

	"github.com/TimelordUK/jqv/internal/query"
	"github.com/itchyny/gojq"
)

// Session is one REPL: the document, the stages pushed so far, and the last line's result.
type Session struct {
	Opt   query.Options
	Width int // a value printed on one line up to this width, pretty beyond it

	root    any
	stages  []string
	last    string // the last line that ran without an error, for :push
	lastVal any    // $_
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

// Do runs one line, jq or a :command, and gives the text to show. quit ends the session.
func (s *Session) Do(line string) (out string, quit bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if strings.HasPrefix(line, ":") {
		return s.command(line)
	}
	return s.eval(line), false
}

func (s *Session) eval(line string) string {
	if _, err := gojq.Parse(line); err != nil { // parse the line alone, so the caret is on it
		return failure(line, err)
	}
	r := s.run(line)
	var b strings.Builder
	for _, v := range r.Values {
		b.WriteString(s.format(v))
		b.WriteByte('\n')
	}
	if r.Err != nil {
		b.WriteString(failure(line, r.Err))
		return b.String()
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
	}
	s.last = line
	if len(r.Values) == 1 && !r.More {
		s.lastVal = r.Values[0]
	} else {
		s.lastVal = append([]any{}, r.Values...)
	}
	return b.String()
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
	if h := query.Hint(err); h != "" {
		b.WriteString("hint: " + h + "\n")
	}
	return b.String()
}

const help = `A line of jq runs on the current . and is forgotten. :push keeps it as a stage.
  :push [expr]  :p   make the last line (or expr) a stage: . is now what it gives
  :pop               take the last stage back
  :clear             drop every stage
  :stages            the stages, with what each gives
  :program           the stages as one jq program, to copy
  :help              this
  :q                 quit (or ctrl+z, enter on Windows; ctrl+d elsewhere)
$root is the whole document; $_ is the last line's result (an array if it gave several).`

func (s *Session) command(line string) (string, bool) {
	name, arg, _ := strings.Cut(line[1:], " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "q", "quit", "exit":
		return "", true
	case "help", "h", "?":
		return help + "\n", false
	case "p", "push":
		return s.push(arg), false
	case "pop":
		if len(s.stages) == 0 {
			return "no stages to pop\n", false
		}
		e := s.stages[len(s.stages)-1]
		s.stages = s.stages[:len(s.stages)-1]
		return "popped: " + e + "\n", false
	case "clear":
		n := len(s.stages)
		s.stages = nil
		return fmt.Sprintf("dropped %d stage%s\n", n, plural(n)), false
	case "stages":
		if len(s.stages) == 0 {
			return "no stages: . is the whole document\n", false
		}
		var b strings.Builder
		all := s.stages
		for i := range all {
			s.stages = all[:i+1]
			fmt.Fprintf(&b, "%d  %s  → %s\n", i+1, all[i], summary(s.run("")))
		}
		s.stages = all
		return b.String(), false
	case "program", "prog":
		return s.Program("") + "\n", false
	}
	return "unknown command :" + name + " (:help lists them)\n", false
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
	return fmt.Sprintf("%d outputs", len(r.Values))
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
// the line typed, then what it gave. It stops at a :q.
func Transcript(s *Session, lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(strings.TrimRight(s.Prompt()+l, " ") + "\n")
		out, quit := s.Do(l)
		b.WriteString(out)
		if quit {
			break
		}
	}
	return b.String()
}
