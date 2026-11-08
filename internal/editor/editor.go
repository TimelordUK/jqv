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

// ErrInterrupted is ReadLine's answer to ctrl+c: the line was abandoned.
var ErrInterrupted = errors.New("interrupted")

// Model reads one line. It is a Bubble Tea model, so a host runs it inline (Reader) or inside
// a larger program, and tests drive it with key messages.
type Model struct {
	Prompt   string
	Keys     Keymap
	Handlers map[Action]Handler
	History  *History
	ShowKeys bool // print each key's name and action as it comes, to see what a terminal sends

	buf   Buffer
	width int
	done  bool
	err   error // io.EOF or ErrInterrupted when done without a line
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

// View is the prompt and the line, wrapped to the terminal's width, with the cursor shown in
// reverse. A finished line is shown plain, with ^C if abandoned, and ends in a newline: Bubble
// Tea clears the line it leaves the cursor on, so that line must be the empty one after it.
func (m *Model) View() string {
	t := m.buf.text
	var s string
	if m.done {
		s = m.Prompt + string(t)
		if m.err == ErrInterrupted {
			s += "^C"
		}
	} else {
		c, rest := " ", ""
		if m.buf.cur < len(t) {
			c, rest = string(t[m.buf.cur]), string(t[m.buf.cur+1:])
		}
		s = m.Prompt + string(t[:m.buf.cur]) + reverse + c + unreverse + rest
	}
	if m.width > 0 {
		s = ansi.Hardwrap(s, m.width, true)
	}
	if m.done {
		s += "\n"
	}
	return s
}

// Reader reads lines with the editor, one inline Bubble Tea program per line, so output
// written between lines scrolls above as it would with a plain reader. History and keys last
// across lines.
type Reader struct {
	In       io.Reader
	Out      io.Writer
	Keys     Keymap
	Handlers map[Action]Handler
	History  History
	ShowKeys bool
}

// ReadLine shows prompt and reads a line: io.EOF on ctrl+d on an empty line, ErrInterrupted
// on ctrl+c.
func (r *Reader) ReadLine(prompt string) (string, error) {
	m := &Model{Prompt: prompt, Keys: r.Keys, Handlers: r.Handlers, History: &r.History, ShowKeys: r.ShowKeys}
	if m.Keys == nil {
		m.Keys = DefaultKeys()
	}
	r.History.reset()
	p := tea.NewProgram(m, tea.WithInput(r.In), tea.WithOutput(r.Out))
	if _, err := p.Run(); err != nil {
		return "", err
	}
	if !m.done { // the program ended without the line finishing
		return "", io.EOF
	}
	return m.Result()
}
