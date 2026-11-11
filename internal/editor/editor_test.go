package editor

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keys turns a script into key messages: plain text is typed, and <name> is a key by its
// Bubble Tea name (<ctrl+w>, <left>, <enter>).
func keys(script string) []tea.KeyMsg {
	var ks []tea.KeyMsg
	for script != "" {
		if strings.HasPrefix(script, "<") {
			if i := strings.Index(script, ">"); i > 1 {
				ks = append(ks, named(script[1:i]))
				script = script[i+1:]
				continue
			}
		}
		r := []rune(script)[0]
		if r == ' ' {
			ks = append(ks, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
		} else {
			ks = append(ks, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		script = script[len(string(r)):]
	}
	return ks
}

func named(n string) tea.KeyMsg {
	if strings.HasPrefix(n, "alt+") && len(n) == 5 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(n[4:]), Alt: true}
	}
	for t, s := range keyNames {
		if s == n {
			return tea.KeyMsg{Type: t}
		}
	}
	panic("no key " + n)
}

var keyNames = map[tea.KeyType]string{}

func init() {
	for _, t := range []tea.KeyType{tea.KeyEnter, tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyCtrlL,
		tea.KeyEsc, tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete, tea.KeyLeft, tea.KeyRight,
		tea.KeyHome, tea.KeyEnd, tea.KeyCtrlA, tea.KeyCtrlE, tea.KeyCtrlLeft, tea.KeyCtrlRight,
		tea.KeyCtrlK, tea.KeyCtrlU, tea.KeyCtrlW, tea.KeyCtrlY, tea.KeyUp, tea.KeyDown,
		tea.KeyF1, tea.KeyF2, tea.KeyF3, tea.KeyTab, tea.KeyShiftTab} {
		keyNames[t] = tea.KeyMsg{Type: t}.String()
	}
}

// typeInto sends the script's keys to m and gives the commands they returned.
func typeInto(m *Model, script string) []tea.Cmd {
	var cmds []tea.Cmd
	for _, k := range keys(script) {
		_, c := m.Update(k)
		cmds = append(cmds, c)
	}
	return cmds
}

// line shows the text with | at the cursor.
func line(m *Model) string {
	r := []rune(m.Text())
	return string(r[:m.Cursor()]) + "|" + string(r[m.Cursor():])
}

func TestEditing(t *testing.T) {
	for _, c := range []struct{ script, want string }{
		{"abc", "abc|"},
		{"abc<left><left>X", "aX|bc"},
		{"abc<home>X<end>Y", "XabcY|"},
		{"abc<ctrl+a>X<ctrl+e>Y", "XabcY|"},
		{"abc<backspace>", "ab|"},
		{"abc<left><backspace>", "a|c"},
		{"abc<home><delete>", "|bc"},
		{"abc<left><ctrl+d>", "ab|"},
		{".people[].name<ctrl+w>", ".people[].|"},
		{".people[].name<ctrl+w><ctrl+w>", ".|"},
		{".a | .b<ctrl+left>", ".a | .|b"},
		{".a | .b<home><ctrl+right>", ".a| | .b"},
		{"select(.age)<left><left><left><left><ctrl+k>", "select(.|"},
		{"abc def<left><left><left><ctrl+u>", "|def"},
		{"abc def<ctrl+w><home><ctrl+y>", "def|abc "},
		{"abc<esc>", "|"},
		{"a b", "a b|"},
		{"x<alt+d>", "x|"},  // unbound alt keys type nothing
		{"x<tab>y", "xy|"},  // tab does nothing until a host binds it
		{"é<left>ü", "ü|é"}, // runes, not bytes
	} {
		m := New("> ")
		typeInto(m, c.script)
		if got := line(m); got != c.want {
			t.Errorf("%q: got %q, want %q", c.script, got, c.want)
		}
	}
}

func TestSubmitAndEnd(t *testing.T) {
	m := New("> ")
	typeInto(m, " .a <enter>more")
	if s, err := m.Result(); !m.Done() || err != nil || s != " .a " {
		t.Fatalf("enter: %q %v done=%v", s, err, m.Done())
	}
	if line(m) != " .a |" {
		t.Errorf("keys after enter should do nothing, got %q", line(m))
	}
	if h := m.History.Lines(); len(h) != 1 || h[0] != ".a" {
		t.Errorf("history keeps the line trimmed: %q", h)
	}

	m = New("> ")
	typeInto(m, "<ctrl+d>")
	if _, err := m.Result(); err != io.EOF {
		t.Errorf("ctrl+d on an empty line: %v, want EOF", err)
	}
	m = New("> ")
	typeInto(m, ".a<ctrl+c>")
	if _, err := m.Result(); err != ErrInterrupted {
		t.Errorf("ctrl+c: %v", err)
	}
	if v := m.View(); v != "> .a^C\n" {
		t.Errorf("an abandoned line shows ^C: %q", v)
	}
}

func TestHistory(t *testing.T) {
	h := &History{}
	for _, l := range []string{".a", ".b", ".b", "", ".c"} {
		h.Add(l)
	}
	if got := strings.Join(h.Lines(), " "); got != ".a .b .c" {
		t.Fatalf("no blanks or repeats: %q", got)
	}
	m := New("> ")
	m.History = h
	typeInto(m, "dra<up>")
	if line(m) != ".c|" {
		t.Errorf("up: %q", line(m))
	}
	typeInto(m, "<up><up><up>")
	if line(m) != ".a|" {
		t.Errorf("up stops at the oldest: %q", line(m))
	}
	typeInto(m, "<down><down><down>")
	if line(m) != "dra|" {
		t.Errorf("down past the newest gives back the draft: %q", line(m))
	}
	typeInto(m, "<down>")
	if line(m) != "dra|" {
		t.Errorf("down when not walking does nothing: %q", line(m))
	}
}

func TestHandler(t *testing.T) {
	m := New("> ")
	var got string
	m.Keys["f1"] = "help"
	m.Keys["f2"] = "home"
	m.Handlers = map[Action]Handler{"help": func(line string, cur int) (string, int) {
		got = line[:cur]
		return "help text\n", cur
	}, "home": func(string, int) (string, int) { return "", 0 }}
	cmds := typeInto(m, "trim<left><f1>")
	if got != "tri" {
		t.Errorf("the handler gets the line and cursor: %q", got)
	}
	if cmds[len(cmds)-1] == nil {
		t.Error("a handler's text is printed above the line")
	}
	if line(m) != "tri|m" {
		t.Errorf("the line stays as it was: %q", line(m))
	}
	typeInto(m, "<f2>")
	if line(m) != "|trim" {
		t.Errorf("a handler moves the cursor: %q", line(m))
	}
	typeInto(m, "<f2>")
	if line(m) != "|trim" {
		t.Errorf("a handler moves the cursor: %q", line(m))
	}
}

func TestClearAndPaste(t *testing.T) {
	m := New("> ")
	if c := typeInto(m, "<ctrl+l>")[0]; c == nil {
		t.Error("ctrl+l clears the screen")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".a |\r\n  .b"), Paste: true})
	if line(m) != ".a |   .b|" {
		t.Errorf("paste joins lines: %q", line(m))
	}
}

func TestView(t *testing.T) {
	m := New("> ")
	typeInto(m, "abc<left>")
	if v := m.View(); v != "> ab"+reverse+"c"+unreverse {
		t.Errorf("cursor on c: %q", v)
	}
	typeInto(m, "<end>")
	if v := m.View(); v != "> abc"+reverse+" "+unreverse {
		t.Errorf("cursor at the end: %q", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 4})
	if v := m.View(); strings.Count(v, "\n") != 1 {
		t.Errorf("a line wider than the terminal wraps: %q", v)
	}
	typeInto(m, "<enter>")
	if v := m.View(); !strings.HasSuffix(v, "\n") || strings.Contains(v, reverse) {
		t.Errorf("a finished line is plain and ends in a newline: %q", v)
	}
}

// A Painter's spans are drawn by their styles, the rest plain; the cursor is reversed inside
// its span's style, and a finished line keeps its colour but loses the cursor.
func TestPainter(t *testing.T) {
	tag := func(name string) func(string) string {
		return func(s string) string { return "<" + name + ":" + s + ">" }
	}
	m := New("> ")
	m.Painter = func(line string) []Span {
		return []Span{{0, 2, tag("f")}, {5, 7, tag("n")}}
	}
	typeInto(m, ".a | ab<left>")
	R, U := reverse, unreverse
	for _, c := range []struct{ keys, want string }{
		{"", "> <f:.a> | <n:a>" + R + "<n:b>" + U},
		{"<end>", "> <f:.a> | <n:ab>" + R + " " + U},
		{"<home>", "> " + R + "<f:.>" + U + "<f:a> | <n:ab>"},
		{"<right><right><right>", "> <f:.a> " + R + "|" + U + " <n:ab>"},
	} {
		typeInto(m, c.keys)
		if v := m.View(); v != c.want {
			t.Errorf("after %q:\n got  %q\n want %q", c.keys, v, c.want)
		}
	}
	typeInto(m, "<enter>")
	if v := m.View(); v != "> <f:.a> | <n:ab>\n" {
		t.Errorf("finished: %q", v)
	}
}

// ident is a host's word: here jq's, with its sigils.
func ident(r rune) bool { return isWord(r) || r == '@' || r == '$' }

func TestWordAt(t *testing.T) {
	for _, c := range []struct {
		line string
		cur  int
		want string
	}{
		{".[] | trim", 10, "trim"},
		{".[] | trim", 8, "trim"},
		{"map(ltrim)", 4, "ltrim"},
		{"map(ltrim)", 3, "map"},
		{". | ", 4, ""},
		{"@csv", 2, "@csv"},
		{"$ENV.x", 1, "$ENV"},
	} {
		if got := WordAt(c.line, c.cur, ident); got != c.want {
			t.Errorf("WordAt(%q, %d) = %q, want %q", c.line, c.cur, got, c.want)
		}
	}
}

func TestShowKeys(t *testing.T) {
	if got := keyNote("ctrl+down", "step-on", false); got != "key: ctrl+down → step-on" {
		t.Errorf("%q", got)
	}
	if got := keyNote("down", "", false); got != "key: down (not bound)" {
		t.Errorf("%q", got)
	}
	m := New("> ")
	m.ShowKeys = true
	if c := typeInto(m, "a")[0]; c == nil || line(m) != "a|" {
		t.Errorf("with --show-keys a key still does its work, and is printed: %q", line(m))
	}
}

// TestBareModifierTypesNothing is from daily use (2026-11-08, Windows Terminal): pressing
// ctrl alone comes from Bubble Tea as KeyRunes with a NUL rune, which went into the line,
// unseen, before every ctrl+up, so the stages after it stopped parsing and ctrl+down found
// nowhere to go.
func TestBareModifierTypesNothing(t *testing.T) {
	m := New("> ")
	typeInto(m, "abc<left>")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x1f', '\a'}})
	if got := line(m); got != "ab|c" {
		t.Errorf("control runes typed into the line: %q", got)
	}
}

func TestSubmitter(t *testing.T) {
	r := &Reader{Keys: DefaultKeys(), Submits: map[Action]Submitter{
		"pipe": func(line string, cur int) string { return ":!" },
		"none": func(string, int) string { return "" },
	}}
	r.Keys["f2"], r.Keys["f3"] = "pipe", "none"
	m := r.model("> ")
	typeInto(m, ".a | ke<f3>")
	if m.Done() {
		t.Fatal("a Submitter giving \"\" does nothing")
	}
	typeInto(m, "<left><f2>")
	if s, err := m.Result(); !m.Done() || err != nil || s != ":!" {
		t.Fatalf("f2 submits its own line: %q %v", s, err)
	}
	if v := m.View(); v != "> :!\n" {
		t.Errorf("the finished line shows what ran: %q", v)
	}
	if h := m.History.Lines(); len(h) != 0 {
		t.Errorf("a key's line isn't history: %q", h)
	}
	r.draft = m.kept // as ReadLine does
	if m := r.model("> "); line(m) != ".a | k|e" {
		t.Errorf("the next prompt gives the line back, cursor and all: %q", line(m))
	}
	if m := r.model("> "); m.Text() != "" {
		t.Errorf("only once: %q", m.Text())
	}
}

// words is a completer over a fixed list: the word before the cursor, by prefix.
func words(ws ...string) Completer {
	return func(line string, cur int) (int, int, []Candidate) {
		r := []rune(line)
		from := cur
		for from > 0 && isWord(r[from-1]) {
			from--
		}
		if from == cur {
			return from, cur, nil
		}
		var cs []Candidate
		for _, w := range ws {
			if strings.HasPrefix(w, string(r[from:cur])) {
				cs = append(cs, Candidate{Insert: w, Hint: "about " + w})
			}
		}
		return from, cur, cs
	}
}

func TestComplete(t *testing.T) {
	for _, c := range []struct{ script, want string }{
		{"x co<tab>", "x combinations|"},              // the first
		{"x co<tab><tab>", "x contains|"},             // the next
		{"x co<tab><tab><tab>", "x combinations|"},    // round again
		{"x co<shift+tab>", "x contains|"},            // back from the start: the last
		{"x co<tab><tab><esc>", "x co|"},              // esc: what was typed
		{"x co<tab><esc><esc>", "|"},                  // and esc again empties, as ever
		{"x co<tab>Z", "x combinationsZ|"},            // typing keeps the one shown
		{"x co<tab> <tab>", "x combinations |"},       // a new Tab asks again: nothing at a space
		{"x len<tab>", "x length|"},                   // one: straight in
		{"x co<left><tab>", "x combinations|o"},       // the word before the cursor: c
		{"x zz<tab>", "x zz|"},                        // none: nothing
		{"x co<tab><tab><left><tab>", "x contains|s"}, // a move ends the cycle; then contain is the word
	} {
		m := New("> ")
		m.Completer = words("combinations", "contains", "length")
		typeInto(m, c.script)
		if got := line(m); got != c.want {
			t.Errorf("%q: got %q, want %q", c.script, got, c.want)
		}
	}
}

func TestCompleteView(t *testing.T) {
	m := New("> ")
	m.Completer = words("combinations", "contains", "length")
	typeInto(m, "co<tab><tab>")
	want := "> contains" + reverse + " " + unreverse + "\ncombinations  " + reverse + "contains" + unreverse + "\nabout contains"
	if v := m.View(); v != want {
		t.Errorf("cycling:\n got  %q\n want %q", v, want)
	}
	typeInto(m, "<esc><esc>len<tab>")
	if v := m.View(); v != "> length"+reverse+" "+unreverse+"\nabout length" {
		t.Errorf("one candidate: its hint until the next key: %q", v)
	}
	typeInto(m, "<left>")
	if v := m.View(); strings.Contains(v, "\n") {
		t.Errorf("the hint goes at the next key: %q", v)
	}
}
