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

// The actions assist adds, and their keys: F1 help, F2 the last result into the pipe, F3 the
// stack of what . is at the cursor, F5 run to the cursor, F8 the line laid out, ctrl+up and
// ctrl+down step back and on along the pipeline, ctrl+] to the matching bracket. None is taken
// by Zellij, tmux or Windows Terminal.
const (
	Help     editor.Action = "help"
	Pipe     editor.Action = "pipe"
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
	r.Keys["f1"], r.Handlers[Help] = Help, HelpFor(s)
	r.Keys["f2"], r.Submits[Pipe] = Pipe, PipeLast
	r.Keys["f5"], r.Handlers[RunTo] = RunTo, RunToCursor(s)
	r.Keys["f3"], r.Handlers[Scope] = Scope, Stack(s)
	r.Keys["f8"], r.Rewrites[Layout] = Layout, ToggleLayout(s)
	r.Keys["ctrl+up"], r.Handlers[StepBack] = StepBack, Step(s, -1)
	r.Keys["ctrl+down"], r.Handlers[StepOn] = StepOn, Step(s, +1)
	r.Keys["ctrl+]"], r.Handlers[Match] = Match, MatchBracket
	// the colours ask the terminal whether it is dark: asked now, before the editor reads
	// keys, the answer can't arrive as typing
	lipgloss.HasDarkBackground()
	c := newChecker(s)
	r.Painter = Checked(Colours(), c)              // the line coloured, what the checks found underlined (Q-046)
	r.Hinter = Hints(CheckHint(c), ContextHint(s)) // what a check found at the cursor, else what . is there (Q-047)
	s.Paint = Paint(Colours())
	// Tab (Q-036): the editor cycles, Complete knows what to offer; what shows under the line is dim
	r.Completer = CompleteOn(s)
	r.Continues = Unfinished // enter on an unfinished entry goes on to a new line (Q-026)
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

// PipeLast is F2: a bare :!, the last result into config.toml's [repl] pipe (bat -l json),
// which needs the terminal, so it runs as a line; the line being typed comes back after.
func PipeLast(string, int) string { return ":!" }

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
// a stage a line (jqsyntax.Layout, as :fmt prints it), and F8 again joins it back onto one
// line (jqsyntax.Join). A comment keeps its line break, so an entry with comments joins as far
// as it can, and F8 then lays it out again. The cursor stays on the token it was on. On a
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
			out := jqsyntax.Layout(s.Last())
			return out, len([]rune(out)), ""
		}
		out := jqsyntax.Join(expr)
		if out == expr {
			out = jqsyntax.Layout(expr)
		}
		if out == expr {
			return line, cur, "one stage: nothing to lay out"
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
