package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite golden files")

const sample = `{"name":"jqv","meta":{"v":1,"tags":["a","b"]},"items":[{"qty":2,"note":"long note that will not fit in a narrow pane"},{"qty":null}],"ok":true}`

func testModel(t *testing.T) Model {
	t.Helper()
	root, err := doc.ParseBytes([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	m := New("sample.json", root)
	return send(m, tea.WindowSizeMsg{Width: 40, Height: 10})
}

// key builds the message bubbletea delivers for a key name as KeyMsg.String() spells it.
func key(s string) tea.KeyMsg {
	special := map[string]tea.KeyType{
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "f1": tea.KeyF1, "backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
		"right": tea.KeyRight, "home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp,
		"pgdown": tea.KeyPgDown, "ctrl+d": tea.KeyCtrlD, "ctrl+f": tea.KeyCtrlF, "ctrl+b": tea.KeyCtrlB, "ctrl+g": tea.KeyCtrlG, "ctrl+u": tea.KeyCtrlU, "ctrl+c": tea.KeyCtrlC,
		" ": tea.KeySpace, "space": tea.KeySpace,
	}
	if k, ok := special[s]; ok {
		return tea.KeyMsg{Type: k}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// press sends each space-separated key; "z R" is the chord zR.
func press(m Model, keys string) Model {
	for _, k := range strings.Fields(keys) {
		m = send(m, key(k))
	}
	return m
}

func TestKeyNamesMatchBubbletea(t *testing.T) {
	for _, s := range []string{"enter", "up", "ctrl+d", " ", "j", "G"} {
		if got := key(s).String(); got != s {
			t.Errorf("key(%q).String() = %q", s, got)
		}
	}
}

func TestNavigation(t *testing.T) {
	cases := []struct {
		keys string
		path string
	}{
		{"", "."},
		{"j", ".name"},
		{"j j l", ".meta"},      // l opens a closed container, cursor stays
		{"j j l l", ".meta.v"},  // l again steps into it
		{"j j l l h", ".meta"},  // h on a scalar goes to the parent
		{"j j l h", ".meta"},    // h on an open container closes it
		{"j j l h j", ".items"}, // and the closed children are skipped
		{"G", ".ok"},            // bottom
		{"G g g", "."},          // gg top
		{"k k k", "."},          // clamps at the top
		{"z R G", ".ok"},        // expand all
		{"z R j j j j j j", ".meta.tags[1]"},
		{"z R j j j j j j z M", ".meta"}, // collapse all keeps the cursor on the nearest visible ancestor
		{"j j j enter j", ".items[0]"},   // enter toggles
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
	}
}

func TestBulkOpenCloseFromCursor(t *testing.T) {
	full := press(testModel(t), "z R").Tree().Len()

	m := press(testModel(t), "L") // at the root: the whole document
	if got := m.Tree().Len(); got != full {
		t.Fatalf("L at root: %d rows, want %d", got, full)
	}

	m = press(testModel(t), "j j L") // on .meta: only meta's subtree
	if !m.Tree().Expanded(m.Cursor().Children[1]) || m.Tree().Expanded(m.Tree().Root.Children[2]) {
		t.Fatal("L on .meta should open meta.tags and leave items closed")
	}

	m = press(testModel(t), "z R H") // at the root: an outline of the top level
	if got, want := m.Tree().Len(), testModel(t).Tree().Len(); got != want {
		t.Fatalf("H at root: %d rows, want %d", got, want)
	}

	m = press(testModel(t), "z R j j H") // on .meta: meta open, tags closed, items untouched
	tr := m.Tree()
	if !tr.Expanded(m.Cursor()) || tr.Expanded(m.Cursor().Children[1]) || !tr.Expanded(tr.Root.Children[2]) {
		t.Fatal("H on .meta should keep meta open, close tags, leave items open")
	}
	if m.Cursor().Path() != ".meta" {
		t.Fatalf("cursor moved to %s", m.Cursor().Path())
	}
}

// fingerprint is everything a key can change.
func fingerprint(m Model) string {
	var b strings.Builder
	for _, r := range m.Tree().Rows() {
		b.WriteString(m.Tree().Label(r) + "\n")
	}
	return fmt.Sprint(m.cursor, m.top, m.help, m.pending, m.status, m.prompt, m.input, m.query, m.number, m.relnum, b.String())
}

// TestHelpCoversKeys: every key the help lists does something, from at least one of two
// starting points: .meta fully open, .meta first level only, after a search, low in the document.
func TestHelpCoversKeys(t *testing.T) {
	starts := []string{"z R j j", "j j", "j j / q t y enter", "z R G k k k k k"}
	for _, g := range keymap {
		for _, b := range g.bindings {
			for _, k := range strings.Fields(b.keys) {
				chord := k
				if len(k) == 2 && (k[0] == 'z' || k == "gg") {
					chord = k[:1] + " " + k[1:]
				}
				did := false
				for _, s := range starts {
					before := press(testModel(t), s)
					before.Copy = func(string) error { return nil }
					snap := fingerprint(before)
					after := before
					keys := strings.Fields(chord)
					for i, kk := range keys {
						next, cmd := after.Update(key(kk))
						after = next.(Model)
						if i == len(keys)-1 && cmd != nil {
							did = true // quit
						}
					}
					if fingerprint(after) != snap {
						did = true
					}
				}
				if !did {
					t.Errorf("help lists %q (%s) but it does nothing", k, b.what)
				}
			}
		}
	}
}

func TestHelpOpensAndCloses(t *testing.T) {
	m := press(send(testModel(t), tea.WindowSizeMsg{Width: 80, Height: 40}), "f1")
	if !m.help || !strings.Contains(m.View(), "fold everything under the cursor") {
		t.Fatal("F1 should show the help")
	}
	if m = press(m, "j"); !m.help || m.cursor != 0 {
		t.Fatal("keys other than F1 esc q are ignored while help shows")
	}
	if _, cmd := m.Update(key("q")); cmd != nil {
		t.Fatal("q closes help, it does not quit")
	}
	for _, k := range []string{"f1", "esc", "q"} {
		if got := press(press(testModel(t), "f1"), k); got.help {
			t.Errorf("%s did not close help", k)
		}
	}
}

func TestRootDoesNotCollapse(t *testing.T) {
	m := press(testModel(t), "h enter")
	if !m.Tree().Expanded(m.Tree().Root) {
		t.Fatal("root closed")
	}
}

func TestZDepth(t *testing.T) {
	m := press(testModel(t), "z 3")
	if got := m.Tree().Len(); got != 14 {
		t.Fatalf("rows after z3 = %d, want 14", got)
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := press(testModel(t), "z R G")
	if m.cursor < m.top || m.cursor >= m.top+m.bodyHeight() {
		t.Fatalf("cursor %d outside [%d,%d)", m.cursor, m.top, m.top+m.bodyHeight())
	}
	m = press(m, "g g")
	if m.top != 0 {
		t.Fatalf("top = %d after gg", m.top)
	}
}

func TestYank(t *testing.T) {
	var got string
	m := testModel(t)
	m.Copy = func(s string) error { got = s; return nil }
	m = press(m, "z R j j j j j j y")
	if got != ".meta.tags[1]" || !strings.Contains(m.status, ".meta.tags[1]") {
		t.Fatalf("yank path = %q, status %q", got, m.status)
	}
	m = press(m, "k Y")
	if got != `"a"` {
		t.Fatalf("yank value = %q", got)
	}
	m = press(m, "j") // status is one-shot
	if m.status != "" {
		t.Fatalf("status kept: %q", m.status)
	}
}

func TestQuit(t *testing.T) {
	_, cmd := testModel(t).Update(key("q"))
	if cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q should quit")
	}
}

// TestViewFits: every line fits the width and the view is exactly the window's height.
func TestViewFits(t *testing.T) {
	for _, w := range []int{20, 40, 120} {
		m := send(press(testModel(t), "z R"), tea.WindowSizeMsg{Width: w, Height: 8})
		lines := strings.Split(m.View(), "\n")
		if len(lines) != 8 {
			t.Errorf("width %d: %d lines, want 8", w, len(lines))
		}
		for i, l := range lines {
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d line %d too wide (%d): %q", w, i, ansi.StringWidth(l), l)
			}
		}
	}
}

// TestViewGolden pins the screen layout. Tests run without a terminal, so lipgloss emits
// no colour and the golden file is plain text.
func TestViewGolden(t *testing.T) {
	m := press(testModel(t), "z R j j j")
	got := ansi.Strip(m.View()) + "\n"
	path := filepath.Join("testdata", "view.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("view differs\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestSearch(t *testing.T) {
	cases := []struct {
		keys   string
		path   string
		status string // substring of the status message, "" for none
	}{
		{"/ q t y enter", ".items[0].qty", ""},                                         // opens closed parents to land
		{"/ q t y enter n", ".items[1].qty", ""},                                       // n: next
		{"/ q t y enter n n", ".items[0].qty", "search hit BOTTOM, continuing at TOP"}, // wraps
		{"/ q t y enter n N", ".items[0].qty", ""},                                     // N: back
		{"? q t y enter", ".items[1].qty", "search hit TOP, continuing at BOTTOM"},     // ? from the root wraps backwards
		{"? q t y enter n", ".items[0].qty", ""},                                       // n repeats ?'s direction
		{"? q t y enter N", ".items[0].qty", "BOTTOM"},                                 // N reverses it
		{"/ Q T Y enter", ".", "pattern not found: QTY"},                               // a capital means exact case
		{`/ Q T Y \ c enter`, ".items[0].qty", ""},                                     // \c forces ignore case
		{"/ n o t e enter n", ".items[0].note", "BOTTOM"},                              // key and value both match: one node, so n wraps onto it
		{"/ l o n g space n o t e enter", ".items[0].note", ""},                        // spaces in the pattern
		{"/ q t y enter / enter", ".items[1].qty", ""},                                 // empty pattern repeats the last
		{"/ q t x backspace y enter", ".items[0].qty", ""},                             // backspace edits
		{"/ q t y esc", ".", ""},                                                       // esc cancels
		{"n", ".", "no previous search"},
		{"/ q enter", ".name", ""}, // q while typing is text, not quit ("jqv" matches)
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
		if c.status == "" && m.status != "" || !strings.Contains(m.status, c.status) {
			t.Errorf("%q: status %q, want %q", c.keys, m.status, c.status)
		}
	}
}

func TestSearchStatusAndClear(t *testing.T) {
	m := press(testModel(t), "/ q t y enter")
	if v := m.View(); !strings.Contains(v, "match 1/2") {
		t.Fatalf("want match 1/2 in the status bar:\n%s", v)
	}
	if m = press(m, "k"); !strings.Contains(m.View(), "2 matches") {
		t.Fatal("off a match the bar shows the count")
	}
	if m = press(m, "esc"); !m.query.Empty() || strings.Contains(m.View(), "matches") {
		t.Fatal("esc should clear the search")
	}
	if !strings.Contains(press(testModel(t), "/ q t").View(), "/qt") {
		t.Fatal("the prompt shows what is typed")
	}
}

func TestSearchLandsOnScreen(t *testing.T) {
	m := send(testModel(t), tea.WindowSizeMsg{Width: 40, Height: 5}) // three tree rows
	for _, keys := range []string{"z R / o k enter", "z R G / n a m e enter", "z R / t a g s enter n"} {
		m := press(m, keys)
		if m.cursor < m.top || m.cursor >= m.top+m.bodyHeight() {
			t.Errorf("%q: cursor %d off screen [%d,%d)", keys, m.cursor, m.top, m.top+m.bodyHeight())
		}
	}
}

func TestRecentre(t *testing.T) {
	m := press(testModel(t), "z R G k k k k k") // cursor 8 of 14, body 8 rows
	if got := press(m, "z z").top; got != 4 {
		t.Errorf("zz: top = %d, want 4", got)
	}
	if got := press(m, "z b").top; got != 1 {
		t.Errorf("zb: top = %d, want 1", got)
	}
	if got := press(press(m, "g g j j"), "z t").top; got != 2 {
		t.Errorf("zt: top = %d, want 2", got)
	}
}

func TestFoldToMatches(t *testing.T) {
	m := press(testModel(t), "z R / q t y enter z /")
	if got := m.Cursor().Path(); got != ".items[0].qty" {
		t.Fatalf("cursor at %s, want the first match", got)
	}
	tr := m.Tree()
	meta, items := tr.Root.Children[1], tr.Root.Children[2]
	if tr.Expanded(meta) || !tr.Expanded(items) || !tr.Expanded(items.Children[0]) || !tr.Expanded(items.Children[1]) {
		t.Fatal("want meta closed and the path to every qty open")
	}
	if !strings.Contains(m.status, "showing 2 matches") {
		t.Fatalf("status %q", m.status)
	}
	if m = press(testModel(t), "z /"); m.status != "no previous search" {
		t.Fatalf("z/ with no search: status %q", m.status)
	}
	if m = press(testModel(t), "z R / z z z enter z /"); m.Tree().Len() != 14 || !strings.Contains(m.status, "pattern not found") {
		t.Fatalf("z/ with no matches should leave the tree alone: rows %d, status %q", m.Tree().Len(), m.status)
	}
}

// bigModel is a document with more rows than the screen: 60 numbered items.
func bigModel(t *testing.T) Model {
	t.Helper()
	var b strings.Builder
	b.WriteString("[")
	for i := range 60 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"n":%d}`, i)
	}
	b.WriteString("]")
	root, err := doc.ParseBytes([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return send(New("big.json", root), tea.WindowSizeMsg{Width: 60, Height: 12}) // 10 rows on screen, 61 in all
}

func TestPaging(t *testing.T) {
	m := press(bigModel(t), "ctrl+f")
	if m.top != 8 || m.cursor != 8 {
		t.Fatalf("ctrl+f: top %d cursor %d, want 8 8 (a screen less two rows of context)", m.top, m.cursor)
	}
	if m = press(m, "ctrl+f ctrl+f"); m.top != 24 || m.cursor != 24 {
		t.Fatalf("ctrl+f x3: top %d cursor %d", m.top, m.cursor)
	}
	if m = press(m, "ctrl+b"); m.top != 16 || m.cursor != 16 {
		t.Fatalf("ctrl+b: top %d cursor %d", m.top, m.cursor)
	}
	m = press(m, "pgdown pgdown pgdown pgdown pgdown pgdown pgdown pgdown")
	if m.cursor != 60 || m.top != 51 {
		t.Fatalf("paging past the end: top %d cursor %d, want 51 60", m.top, m.cursor)
	}
	if m = press(m, "pgup pgup pgup pgup pgup pgup pgup pgup pgup"); m.cursor != 0 || m.top != 0 {
		t.Fatalf("paging past the start: top %d cursor %d", m.top, m.cursor)
	}
}

func TestCtrlG(t *testing.T) {
	m := press(bigModel(t), "G ctrl+g")
	want := `"big.json"  row 61 of 61 --100%--  value 120 of 121 (99%)  depth 1`
	if m.status != want {
		t.Fatalf("ctrl+g:\n got %s\nwant %s", m.status, want)
	}
}

func TestCommandLine(t *testing.T) {
	m := press(bigModel(t), ": 4 0 enter")
	if m.cursor != 39 || m.cursor < m.top || m.cursor >= m.top+m.bodyHeight() {
		t.Fatalf(":40 -> cursor %d top %d", m.cursor, m.top)
	}
	if m = press(m, ": 9 9 9 enter"); m.cursor != 60 {
		t.Fatalf(":999 clamps to the last row, cursor %d", m.cursor)
	}
	if m = press(m, ": x y z enter"); m.status != "not a command: xyz" {
		t.Fatalf("status %q", m.status)
	}
	if _, cmd := press(m, ": q").Update(key("enter")); cmd == nil {
		t.Fatal(":q should quit")
	}
	if m = press(m, ": 4 esc"); m.prompt != "" || m.cursor != 60 {
		t.Fatal("esc cancels a command")
	}
}

func TestRowNumbers(t *testing.T) {
	m := press(bigModel(t), ": s e t space n u enter j j")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.HasPrefix(lines[1], " 1 ") || !strings.HasPrefix(lines[3], " 3 ") {
		t.Fatalf(":set nu gutter:\n%s", strings.Join(lines[1:4], "\n"))
	}
	m = press(m, ": s e t space r n u enter")
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.HasPrefix(lines[1], " 2 ") || !strings.HasPrefix(lines[3], " 3 ") || !strings.HasPrefix(lines[4], " 1 ") {
		t.Fatalf("nu + rnu: cursor row absolute, others relative:\n%s", strings.Join(lines[1:5], "\n"))
	}
	m = press(m, ": s e t space n o n u enter")
	if strings.HasPrefix(ansi.Strip(m.View()), " 1") || m.number || m.relnum {
		t.Fatal(":set nonu turns the gutter off")
	}
	for _, w := range []int{20, 60} { // nothing overflows with the gutter on
		m := send(press(bigModel(t), ": s e t space n u enter"), tea.WindowSizeMsg{Width: w, Height: 12})
		for i, l := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d line %d is %d wide", w, i, ansi.StringWidth(l))
			}
		}
	}
}
