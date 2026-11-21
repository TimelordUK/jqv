// Package assist is what the line editor knows about jq (DESIGN §18): handlers that plug into
// internal/editor, which knows none. Each gets the line and the cursor and runs things on a
// bench.Session; none touches the screen, so each is tested alone.
package assist

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/charmbracelet/lipgloss"
)

// The actions assist adds. Their keys are Defaults (bindings.go), which config.toml's [keys]
// can change.
const (
	Help     editor.Action = "help"
	Scope    editor.Action = "scope"
	RunTo    editor.Action = "run-to-cursor"
	Layout   editor.Action = "layout"
	StepBack editor.Action = "step-back"
	StepOn   editor.Action = "step-on"
	Match    editor.Action = "match-bracket"
)

// Bind adds the actions to r's keys, on s.
func Bind(r *editor.Reader, s *bench.Session) {
	if r.Handlers == nil {
		r.Handlers = map[editor.Action]editor.Handler{}
	}
	if r.Submits == nil {
		r.Submits = map[editor.Action]editor.Submitter{}
	}
	if r.Rewrites == nil {
		r.Rewrites = map[editor.Action]editor.Rewriter{}
	}
	if r.Previews == nil {
		r.Previews = map[editor.Action]editor.Handler{}
	}
	r.Handlers[Help] = HelpFor(s)
	r.Handlers[Scope] = Stack(s)
	r.Rewrites[Layout] = ToggleLayout(s)
	r.Handlers[Match] = MatchBracket
	// running up to the cursor shows under the line, redrawn as you step (Q-054, DESIGN §24)
	r.Previews[RunTo] = RunToCursor(s)
	r.Previews[StepBack] = Step(s, -1)
	r.Previews[StepOn] = Step(s, +1)
	for _, d := range Defaults {
		if err := bindKey(r, d.Key, d.To); err != nil {
			panic(err) // a default that doesn't bind is a bug here, caught by any test
		}
	}
	// the colours ask the terminal whether it is dark: asked now, before the editor reads
	// keys, the answer can't arrive as typing
	lipgloss.HasDarkBackground()
	c := newChecker(s)
	r.Painter = Checked(Colours(), c)                             // the line coloured, what the checks found underlined (Q-046)
	r.Hinter = Hints(CheckHint(c), RecursionHint, ContextHint(s)) // what a check found at the cursor, recursion (Q-052), else what . is there (Q-047)
	s.Paint = Paint(Colours())
	// Tab (Q-036): the editor cycles, Complete knows what to offer; what shows under the line is dim
	r.Completer = CompleteOn(s)
	r.StateOf = s.State                 // F12: what the editor and the session know, to debug
	r.Entry = func() editor.EntryKind { // what is out for editing changes a few keys (Q-049)
		switch {
		case s.EditingWhole():
			return editor.EntryProgram
		case s.Editing() != "":
			return editor.EntryUnit
		}
		return editor.EntryLine
	}
	r.Continues = func(text string) string { // enter on an unfinished entry goes on to a new line (Q-026)
		if s.Editing() != "" { // but a stage out in the editor ends wanting more (`… as $x`): enter puts it back
			return ""
		}
		return Unfinished(text)
	}
	r.HintStyle = render(lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#7F849C"}))
}

// Unfinished is enter's Continues (Q-026, DESIGN §23): on jq (a line, or a command's jq) that
// leaves something open, what: `open: select( {`, `| at the end`. A closer that closes the
// wrong bracket is a mistake, not unfinished, so enter runs it and its hint says so.
func Unfinished(line string) string {
	_, expr, ok := jqPart(line)
	if !ok || strings.TrimSpace(expr) == "" {
		return ""
	}
	var open, parts []string
	for _, o := range jqsyntax.Unfinished(expr) {
		switch {
		case o.Stray >= 0:
			return ""
		case o.Close == "":
			parts = append(parts, o.Text+" at the end")
		default:
			open = append(open, o.Text)
		}
	}
	if len(open) > 0 {
		parts = append([]string{"open: " + strings.Join(open, " ")}, parts...)
	}
	return strings.Join(parts, " · ")
}

// HelpFor is F1: :help for the word under the cursor, or the REPL's :help on none.
func HelpFor(s *bench.Session) editor.Handler {
	return func(line string, cur int) (string, int) {
		w := editor.WordAt(line, cur, word)
		if w == "" {
			return s.Do(":help").Text, cur
		}
		return s.Do(":help " + w).Text, cur
	}
}

// word is a rune of a jq name: letters, digits and _, and the sigils of @csv and $ENV.
func word(r rune) bool {
	return r == '_' || r == '@' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// RunToCursor is F5: the line run up to the cursor (jqsyntax.UpTo), so a pipeline can be
// looked at stage by stage, mistakes after the cursor and all. It runs as a line does, so
// :push then keeps it.
func RunToCursor(s *bench.Session) editor.Handler {
	return func(line string, cur int) (string, int) {
		return run(s, line, cur), cur
	}
}

// Step is ctrl+up (dir -1) and ctrl+down (+1): the cursor to the end of the stage before or
// after it (jqsyntax.Stops), and the line run up to there. Past the first or last stage it
// stays.
func Step(s *bench.Session, dir int) editor.Handler {
	return func(line string, cur int) (string, int) {
		off, expr, ok := jqPart(line)
		if !ok {
			return notJQ, cur
		}
		stops := jqsyntax.Stops(expr)
		to := -1
		for _, st := range stops {
			st += off
			if dir < 0 && st < cur {
				to = st
			}
			if dir > 0 && st > cur {
				to = st
				break
			}
		}
		if to < 0 {
			return "", cur
		}
		return run(s, line, to), to
	}
}

const notJQ = "F5 and ctrl+up/down run jq up to the cursor; this line is a command\n"

func run(s *bench.Session, line string, cur int) string {
	off, expr, ok := jqPart(line)
	if !ok {
		return notJQ
	}
	p := jqsyntax.UpTo(expr, max(cur-off, 0))
	return "▸ " + p + "\n" + s.Do(p).Text
}

// withExpr are the commands whose argument is jq.
var withExpr = regexp.MustCompile(`^(\s*:(?:p|push|pp|tree|lex|ast|fmt)\s+)`)

// jqPart is the jq in a line and where it starts, in runes: the whole line, or the argument
// of :push, :pp, :tree, :lex or :fmt. ok is false for other commands.
func jqPart(line string) (off int, expr string, ok bool) {
	if m := withExpr.FindString(line); m != "" {
		return len([]rune(m)), line[len(m):], true
	}
	if strings.HasPrefix(strings.TrimSpace(line), ":") {
		return 0, "", false
	}
	return 0, line, true
}

// ToggleLayout is F8 (Q-026, owner 2026-11-15): the entry laid out over lines in the editor,
// a stage a line, groups opened only when too wide for the editor (jqsyntax.Layout, as :fmt
// prints it), and F8 on what is laid out already joins it back onto one line (jqsyntax.Join).
// An entry written over lines by hand is laid out from its own text, comments and blank lines
// kept, rather than joined first (owner 2026-11-18, on :e main). A comment keeps its line
// break, so an entry with comments joins as far as it can. The cursor stays on its token. On a
// command that takes jq (:push expr), its jq; on an empty line, the last line run, laid out.
// :fmt still prints a layout without touching the line.
func ToggleLayout(s *bench.Session) editor.Rewriter {
	return func(line string, cur int) (string, int, string) {
		off, expr, ok := jqPart(line)
		switch {
		case !ok:
			return line, cur, "F8 lays out jq: a line, or the jq after :push, :pp, :tree or :fmt"
		case strings.TrimSpace(line) == "":
			if s.Last() == "" {
				return line, cur, "nothing run yet: F8 lays out the line being typed, or the last line run"
			}
			out := jqsyntax.Layout(s.Last(), s.LayoutWidth())
			return out, len([]rune(out)), ""
		}
		// laid out from the text as it is, keeping its comments and blank lines; what is laid out
		// already (by F8, or so written) is joined
		out := jqsyntax.Layout(expr, s.LayoutWidth())
		if out == expr {
			out = jqsyntax.Join(expr)
		}
		if out == expr {
			return line, cur, "nothing to lay out: one stage, and it fits on a line"
		}
		prefix := string([]rune(line)[:off])
		return prefix + out, off + sameToken(expr, out, cur-off), ""
	}
}

// sameToken is where cursor, in from, is in to, when the two differ only in their spaces:
// before the same non-space rune, or at the end.
func sameToken(from, to string, cursor int) int {
	n := 0 // the non-space runes before the cursor
	for i, r := range []rune(from) {
		if i >= cursor {
			break
		}
		if !unicode.IsSpace(r) {
			n++
		}
	}
	rs := []rune(to)
	for i, r := range rs {
		if !unicode.IsSpace(r) {
			if n == 0 {
				return i
			}
			n--
		}
	}
	return len(rs)
}
