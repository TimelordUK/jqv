package editor

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Handler is a host's own action, such as help on the word under the cursor. It gets the line
// and the cursor, and gives text to print above the editor and where the cursor goes; the
// text of the line stays as it is.
type Handler func(line string, cursor int) (out string, moveTo int)

// Submitter is a host's action that runs a line of its own, such as a command that needs the
// terminal to itself: it gets the line and cursor and gives the line to submit instead. The
// line being typed is kept, and a Reader gives it back at the next prompt. "" does nothing.
type Submitter func(line string, cursor int) (run string)

// Span is a run of the line, From to To in runes, drawn by Style (which wraps the text, in
// colour on a terminal and in markers in a test).
type Span struct {
	From, To int
	Style    func(string) string
}

// Painter gives the spans to draw a line with, in order and apart; what none covers is drawn
// plain. The editor knows nothing of what they mean: a host's painter does (assist's knows jq).
type Painter func(line string) []Span

// ErrInterrupted is ReadLine's answer to ctrl+c: the line was abandoned.
var ErrInterrupted = errors.New("interrupted")

// Model reads one line. It is a Bubble Tea model, so a host runs it inline (Reader) or inside
// a larger program, and tests drive it with key messages.
type Model struct {
	Prompt   string
	Keys     Keymap
	Handlers map[Action]Handler
	Submits  map[Action]Submitter
	History  *History
	ShowKeys bool    // print each key's name and action as it comes, to see what a terminal sends
	Painter  Painter // colour for the line, or nil

	buf   Buffer
	kept  *Buffer // the line being typed when a Submitter ran another
	width int
	done  bool
	err   error // io.EOF or ErrInterrupted when done without a line
}

// Kept is the line that was being typed when a Submitter ran a line of its own instead, and
// its cursor; ok is false when the line finished in the ordinary way.
func (m *Model) Kept() (line string, cursor int, ok bool) {
	if m.kept == nil {
		return "", 0, false
	}
	return m.kept.Text(), m.kept.Cursor(), true
}

// New is a model with the default keys and its own history.
func New(prompt string) *Model {
	return &Model{Prompt: prompt, Keys: DefaultKeys(), History: &History{}}
}

// Text and Cursor are the line being edited and where the cursor is in it, in runes.
func (m *Model) Text() string { return m.buf.Text() }
func (m *Model) Cursor() int  { return m.buf.Cursor() }

// Done says whether the line is finished; Result gives it, or io.EOF or ErrInterrupted.
func (m *Model) Done() bool { return m.done }
func (m *Model) Result() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.buf.Text(), nil
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if m.done {
			return m, nil
		}
		if m.ShowKeys {
			name, act := msg.String(), m.Keys[msg.String()]
			return m, tea.Sequence(tea.Println(keyNote(name, act, msg.Paste)), m.key(msg))
		}
		return m, m.key(msg)
	default:
		// a sequence Bubble Tea doesn't know comes as its unknown…Msg, not a key
		if t := fmt.Sprintf("%T", msg); m.ShowKeys && strings.Contains(strings.ToLower(t), "unknown") {
			return m, tea.Println(fmt.Sprintf("key: a sequence Bubble Tea doesn't know: %q", fmt.Sprint(msg)))
		}
	}
	return m, nil
}

func (m *Model) key(k tea.KeyMsg) tea.Cmd {
	if k.Paste { // one line for now: jq doesn't care about line breaks (Q-026 keeps them)
		s := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(string(k.Runes))
		m.buf.Insert([]rune(s)...)
		return nil
	}
	if a, ok := m.Keys[k.String()]; ok {
		return m.do(a)
	}
	switch k.Type {
	case tea.KeyRunes:
		if !k.Alt {
			m.buf.Insert(printable(k.Runes)...)
		}
	case tea.KeySpace:
		m.buf.Insert(' ')
	}
	return nil
}

func (m *Model) do(a Action) tea.Cmd {
	b := &m.buf
	switch a {
	case Submit:
		m.History.Add(strings.TrimSpace(b.Text()))
		return m.finish(nil)
	case Interrupt:
		return m.finish(ErrInterrupted)
	case DeleteOrEOF:
		if len(b.text) == 0 {
			return m.finish(io.EOF)
		}
		b.Delete()
	case Clear:
		return tea.ClearScreen
	case Revert:
		b.Set("")
		m.History.reset()
	case Backspace:
		b.Backspace()
	case Delete:
		b.Delete()
	case Left:
		b.Left()
	case Right:
		b.Right()
	case Home:
		b.Home()
	case End:
		b.End()
	case WordLeft:
		b.WordLeft()
	case WordRight:
		b.WordRight()
	case KillToEnd:
		b.KillToEnd()
	case KillToStart:
		b.KillToStart()
	case KillWordBack:
		b.KillWordBack()
	case KillWordForward:
		b.KillWordForward()
	case Yank:
		b.Yank()
	case HistoryPrev:
		if s, ok := m.History.Prev(b.Text()); ok {
			b.Set(s)
		}
	case HistoryNext:
		if s, ok := m.History.Next(); ok {
			b.Set(s)
		}
	default:
		if sub := m.Submits[a]; sub != nil {
			if run := sub(b.Text(), b.Cursor()); run != "" {
				kept := *b
				m.kept = &kept
				b.Set(run) // shown as the finished line, so the transcript says what ran
				return m.finish(nil)
			}
			return nil
		}
		if h := m.Handlers[a]; h != nil {
			out, to := h(b.Text(), b.Cursor())
			b.cur = min(max(to, 0), len(b.text))
			if out != "" {
				return tea.Println(strings.TrimRight(out, "\n"))
			}
		}
	}
	return nil
}

// printable drops control runes from typed keys. On Windows a modifier pressed alone (ctrl
// before ctrl+up) comes as KeyRunes with a NUL rune; typed into the line, unseen, it broke it.
func printable(rs []rune) []rune {
	out := rs[:0:0]
	for _, r := range rs {
		if !unicode.IsControl(r) {
			out = append(out, r)
		}
	}
	return out
}

// keyNote is --show-keys' line for a key: its name and what it is bound to. A name with
// anything unprintable in it is quoted, so a NUL shows as "\x00", not as nothing.
func keyNote(name string, a Action, paste bool) string {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 || name == "" {
		name = strconv.Quote(name)
	}
	switch {
	case paste:
		return "key: (a paste)"
	case a != "":
		return "key: " + name + " → " + string(a)
	}
	return "key: " + name + " (not bound)"
}

func (m *Model) finish(err error) tea.Cmd {
	m.done, m.err = true, err
	return tea.Quit
}

const (
	reverse   = "\x1b[7m"
	unreverse = "\x1b[27m"
)

// View is the prompt and the line, painted, wrapped to the terminal's width, with the cursor
// shown in reverse. A finished line has no cursor, ^C if abandoned, and ends in a newline:
// Bubble Tea clears the line it leaves the cursor on, so that line must be the empty one after.
func (m *Model) View() string {
	var spans []Span
	if m.Painter != nil {
		spans = m.Painter(m.buf.Text())
	}
	var s string
	if m.done {
		s = m.Prompt + Render(m.buf.text, spans, -1)
		if m.err == ErrInterrupted {
			s += "^C"
		}
	} else {
		s = m.Prompt + Render(m.buf.text, spans, m.buf.cur)
	}
	if m.width > 0 {
		s = ansi.Hardwrap(s, m.width, true)
	}
	if m.done {
		s += "\n"
	}
	return s
}

// Render draws t with spans, and the cursor at cur in reverse (none if cur < 0; past the end
// it is a space). The cursor's rune keeps its span's style inside the reverse.
func Render(t []rune, spans []Span, cur int) string {
	var b strings.Builder
	at := 0
	draw := func(to int, style func(string) string) {
		for at < to {
			if at == cur {
				b.WriteString(reverse + paint(style, string(t[at])) + unreverse)
				at++
				continue
			}
			end := to
			if at < cur && cur < to {
				end = cur
			}
			b.WriteString(paint(style, string(t[at:end])))
			at = end
		}
	}
	for _, sp := range spans {
		from, to := min(max(sp.From, at), len(t)), min(max(sp.To, at), len(t))
		draw(from, nil)
		draw(to, sp.Style)
	}
	draw(len(t), nil)
	if cur >= len(t) {
		b.WriteString(reverse + " " + unreverse)
	}
	return b.String()
}

func paint(style func(string) string, s string) string {
	if style == nil {
		return s
	}
	return style(s)
}

// Reader reads lines with the editor, one inline Bubble Tea program per line, so output
// written between lines scrolls above as it would with a plain reader. History and keys last
// across lines.
type Reader struct {
	In       io.Reader
	Out      io.Writer
	Keys     Keymap
	Handlers map[Action]Handler
	Submits  map[Action]Submitter
	History  History
	ShowKeys bool
	Painter  Painter

	draft *Buffer // a line kept by a Submitter, for the next prompt
}

// ReadLine shows prompt and reads a line: io.EOF on ctrl+d on an empty line, ErrInterrupted
// on ctrl+c.
func (r *Reader) ReadLine(prompt string) (string, error) {
	m := r.model(prompt)
	p := tea.NewProgram(m, tea.WithInput(r.In), tea.WithOutput(r.Out))
	if _, err := p.Run(); err != nil {
		return "", err
	}
	r.draft = m.kept
	if !m.done { // the program ended without the line finishing
		return "", io.EOF
	}
	return m.Result()
}

// model is the next line's model, starting from the line a Submitter kept, if any.
func (r *Reader) model(prompt string) *Model {
	m := &Model{Prompt: prompt, Keys: r.Keys, Handlers: r.Handlers, Submits: r.Submits,
		History: &r.History, ShowKeys: r.ShowKeys, Painter: r.Painter}
	if m.Keys == nil {
		m.Keys = DefaultKeys()
	}
	if r.draft != nil {
		m.buf, r.draft = *r.draft, nil
	}
	r.History.reset()
	return m
}
