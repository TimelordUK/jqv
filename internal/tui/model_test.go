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
	return fmt.Sprint(m.cursor, m.top, m.help, m.pending, m.status, m.prompt, m.input, m.query, m.number, m.relnum, m.lock, b.String())
}

// TestHelpCoversKeys: every key the help lists does something, from at least one of two
// starting points: .meta fully open, .meta first level only, after a search, low in the document.
func TestHelpCoversKeys(t *testing.T) {
	starts := []string{"z R j j", "j j", "j j / q t y enter", "z R G k k k k k"}
	for _, g := range keymap {
		for _, b := range g.bindings {
			for _, k := range strings.Fields(b.keys) {
				chord := k
				if len(k) == 2 && (k[0] == 'z' || k[0] == '[' || k[0] == ']' || k == "gg") {
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

// The owner's flow on the FIX dictionary: search a message by name, get to the top of its block,
// bounce to the end and back, then walk to the neighbouring messages.
func TestBlockWalkOnFIX(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "FIX4.4.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	m := send(New("FIX4.4.json", root), tea.WindowSizeMsg{Width: 80, Height: 30})
	msgtype := func(m Model) string { return m.Cursor().Children[len(m.Cursor().Children)-2].Raw }

	m = press(m, "/ t r a d e c a p t u r e r e p o r t enter n") // Request, then the report itself
	if m.Cursor().Key != "_name" || m.Cursor().Raw != "TradeCaptureReport" {
		t.Fatalf("search landed on %s = %q", m.Cursor().Path(), m.Cursor().Raw)
	}
	m = press(m, "[ {")
	start := m.Cursor()
	if msgtype(m) != "AE" {
		t.Fatalf("[{ landed on %s", start.Path())
	}
	m = press(m, "%")
	if m.Cursor().Key != "_msgcat" || m.Cursor().Parent != start {
		t.Fatalf("%% landed on %s", m.Cursor().Path())
	}
	if m = press(m, "%"); m.Cursor() != start {
		t.Fatalf("%% back landed on %s", m.Cursor().Path())
	}
	m = press(m, "}")
	if m.Cursor().Index != start.Index+1 || !m.Tree().Expanded(m.Cursor()) || m.top != m.cursor {
		t.Fatalf("} landed on %s, open %v, top %d cursor %d", m.Cursor().Path(), m.Tree().Expanded(m.Cursor()), m.top, m.cursor)
	}
	if m = press(m, "{"); m.Cursor() != start {
		t.Fatalf("{ landed on %s", m.Cursor().Path())
	}
}

func TestEnclosingBlock(t *testing.T) {
	cases := []struct {
		keys, path, status string
	}{
		{"z R G k [ {", ".items[1]", ""},       // .items[1].qty
		{"z R j j j j j j [ {", ".meta", ""},   // .meta.tags[1]: skips the array, the nearest {
		{"z R j j [ {", ".", ""},               // on an open { it is the outer one, as in vim
		{"[ {", ".", "no enclosing {"},         // nothing above the root
		{"z R j j ] }", ".ok", ""},             // likewise the outer }
		{"z R j j j ] }", ".meta.tags[1]", ""}, // .meta.v: the last row of .meta
		{"z R j j j ] } ] }", ".ok", ""},       // already at .meta's end: the outer one
		{"z R G ] }", ".ok", "no enclosing }"}, // already at the end of everything
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
		if m.status != c.status {
			t.Errorf("%q: status %q, want %q", c.keys, m.status, c.status)
		}
	}
}

func TestMatchBlock(t *testing.T) {
	cases := []struct {
		keys, path string
	}{
		{"z R j j %", ".meta.tags[1]"},       // open block: its last row, however deep
		{"z R j j % %", ".meta"},             // and back, not to the nearer .meta.tags
		{"z R j j % % %", ".meta.tags[1]"},   // and it keeps bouncing
		{"z R j j j ] } %", ".meta"},         // ]} remembers the block too
		{"z R j j j %", ".meta"},             // a scalar: the start of its block
		{"z R j j j j %", ".meta.tags[1]"},   // the nested block, not the enclosing one
		{"j j %", ".meta.tags"},              // closed: opens, and its end here is a closed row
		{"j j % %", ".meta"},                 // which is the end, so it comes back
		{"j %", "."},                         // a scalar at the top level
		{"z R j j % k k %", ".meta.tags[1]"}, // the memory only applies at the end row
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
	}
}

func TestSiblingBlocks(t *testing.T) {
	cases := []struct {
		keys, path, status string
	}{
		{"j j }", ".items", ""},                // skips the scalars between blocks
		{"j j j l l }", ".items[1]", ""},       // through an array, one element at a time
		{"j j j l l } {", ".items[0]", ""},     // and back
		{"z R j j j }", ".items", ""},          // a scalar counts as the block it is in
		{"j j j }", ".items", "no next block"}, // stops at the last and stays put
		{"j j {", ".meta", "no previous block"},
		{"}", ".", "no next block"},       // the root has no siblings
		{"j }", ".name", "no next block"}, // nor has the block a top-level scalar is in
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
		if m.status != c.status {
			t.Errorf("%q: status %q, want %q", c.keys, m.status, c.status)
		}
	}
	m := press(testModel(t), "j j l }")
	if !m.Tree().Expanded(m.Cursor()) {
		t.Fatal("} should open the block it lands on")
	}
	if !m.Tree().Expanded(m.Tree().Root.Children[1]) {
		t.Fatal("} should leave the block behind it open")
	}
}

// ) and ( iterate the array the cursor is in, whatever depth inside the element it is at.
func TestArrayElements(t *testing.T) {
	cases := []struct {
		keys, path, status string
	}{
		{"j j j )", ".items[0]", ""},                   // on the array: into its first element
		{"j j j (", ".items[1]", ""},                   // or its last
		{"j j j ) )", ".items[1]", ""},                 // element to element
		{"j j j ) ) (", ".items[0]", ""},               // and back
		{"j j j ) ) )", ".items[1]", "last element"},   // stops at the end of this array
		{"j j j ) (", ".items[0]", "first element"},    // and at the start
		{"z R j j j j j )", ".meta.tags[1]", ""},       // scalar elements are elements too
		{"z R G k )", ".items[1].qty", "last element"}, // from a field: the array around it
		{"z R j j j j j j j j )", ".items[1]", ""},     // on an element: the next one, not into it
		{"j )", ".name", "not in an array"},            // nothing to iterate
		{"j j )", ".meta", "not in an array"},
	}
	for _, c := range cases {
		m := press(testModel(t), c.keys)
		if got := m.Cursor().Path(); got != c.path {
			t.Errorf("%q: cursor at %s, want %s", c.keys, got, c.path)
		}
		if m.status != c.status {
			t.Errorf("%q: status %q, want %q", c.keys, m.status, c.status)
		}
	}
}

// The owner's flow on the Nobel prizes: step through prizes, and through the laureates of one.
func TestArrayWalkOnNobel(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "nobel_prize.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	m := send(New("nobel_prize.json", root), tea.WindowSizeMsg{Width: 80, Height: 30})

	steps := []struct{ keys, path string }{
		{"j )", ".prizes[0]"},
		{")", ".prizes[1]"},
		{")", ".prizes[2]"},
		{"(", ".prizes[1]"},
		{"j j j", ".prizes[1].laureates"},
		{")", ".prizes[1].laureates[0]"}, // now iterating the sub-array
		{")", ".prizes[1].laureates[1]"},
		{"j j", ".prizes[1].laureates[1].firstname"},
		{"(", ".prizes[1].laureates[0]"}, // from a field of a laureate, the laureates
	}
	for _, s := range steps {
		m = press(m, s.keys)
		if got := m.Cursor().Path(); got != s.path {
			t.Fatalf("%q: cursor at %s, want %s", s.keys, got, s.path)
		}
		if n := m.Cursor(); !strings.Contains(s.keys, "j j") && !m.Tree().Expanded(n) {
			t.Fatalf("%q: %s should be open", s.keys, n.Path())
		}
	}
}

// zl holds the cursor on the middle row while the document moves under it.
func TestScrollLock(t *testing.T) {
	long := `[` + strings.Repeat(`1,`, 99) + `1]`
	root, err := doc.ParseBytes([]byte(long))
	if err != nil {
		t.Fatal(err)
	}
	m := send(New("long.json", root), tea.WindowSizeMsg{Width: 40, Height: 12}) // 10 rows
	mid := 5

	m = press(m, "z l")
	if !m.lock || !strings.Contains(m.View(), "locked") {
		t.Fatal("zl should lock, and the status bar should say so")
	}
	for _, keys := range []string{"j j j", "j j j j j j j j j", "k", "ctrl+d", "ctrl+u", ":", "G k k k k k k k k k"} {
		if keys == ":" {
			m = press(m, ": 5 0 enter")
		} else {
			m = press(m, keys)
		}
		if got := m.cursor - m.top; m.cursor >= mid && m.cursor < m.tree.Len()-mid && got != mid {
			t.Errorf("after %q: cursor on screen row %d, want %d", keys, got, mid)
		}
	}
	if m = press(m, "/ 1 enter"); m.cursor-m.top != mid {
		t.Errorf("after a search: cursor on screen row %d, want %d", m.cursor-m.top, mid)
	}
	if m = press(m, "g g"); m.top != 0 || m.cursor != 0 { // the ends stop it, as in vim
		t.Errorf("top of the document: top %d cursor %d", m.top, m.cursor)
	}
	if m = press(m, "G"); m.top != m.tree.Len()-10 {
		t.Errorf("bottom of the document: top %d", m.top)
	}

	m = press(m, "g g : 5 0 enter z t") // asking for another position ends the lock
	if m.lock || m.cursor != m.top {
		t.Errorf("zt: lock %v, top %d cursor %d", m.lock, m.top, m.cursor)
	}
	m = press(m, "z l z l")
	if m.lock || strings.Contains(m.View(), "locked") {
		t.Fatal("zl again should unlock")
	}
}
