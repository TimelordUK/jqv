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
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
		"right": tea.KeyRight, "home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp,
		"pgdown": tea.KeyPgDown, "ctrl+d": tea.KeyCtrlD, "ctrl+u": tea.KeyCtrlU, "ctrl+c": tea.KeyCtrlC,
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
	return fmt.Sprint(m.cursor, m.top, m.help, m.pending, m.status, b.String())
}

// TestHelpCoversKeys: every key the help lists does something, from at least one of two
// starting points (.meta with the document fully open, and .meta with only the first level).
func TestHelpCoversKeys(t *testing.T) {
	starts := []string{"z R j j", "j j"}
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
	m := press(send(testModel(t), tea.WindowSizeMsg{Width: 80, Height: 40}), "?")
	if !m.help || !strings.Contains(m.View(), "fold everything under the cursor") {
		t.Fatal("? should show the help")
	}
	if m = press(m, "j"); !m.help || m.cursor != 0 {
		t.Fatal("keys other than ? esc q are ignored while help shows")
	}
	if _, cmd := m.Update(key("q")); cmd != nil {
		t.Fatal("q closes help, it does not quit")
	}
	for _, k := range []string{"?", "esc", "q"} {
		if got := press(press(testModel(t), "?"), k); got.help {
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
