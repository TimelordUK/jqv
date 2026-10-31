package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func fromJSON(t *testing.T, s string) Model {
	t.Helper()
	root, err := doc.ParseBytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return send(New("test.json", root), tea.WindowSizeMsg{Width: 40, Height: 16})
}

func fixture(t *testing.T, name string) Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return fromJSON(t, string(b))
}

// type_ sends each character of s to the prompt, as the terminal would.
func type_(m Model, s string) Model {
	for _, r := range s {
		m = send(m, key(string(r)))
	}
	return m
}

// V-013: the cursor moves as the pattern is typed, and backs out of what it opened.
func TestIncrementalSearch(t *testing.T) {
	m := press(testModel(t), "/ q")
	if got := m.Cursor().Path(); got != ".name" { // "jqv" matches first
		t.Fatalf("after /q cursor at %s", got)
	}
	m = press(m, "t")
	if got := m.Cursor().Path(); got != ".items[0].qty" {
		t.Fatalf("after /qt cursor at %s", got)
	}
	if !m.Tree().Expanded(m.Tree().Root.Children[2]) {
		t.Fatal("the preview should open the way to the match")
	}
	if !strings.Contains(m.View(), "2 matches") {
		t.Fatalf("want the count while typing:\n%s", m.View())
	}
	m = press(m, "backspace")
	if got := m.Cursor().Path(); got != ".name" {
		t.Fatalf("after backspace cursor at %s", got)
	}
	if m.Tree().Expanded(m.Tree().Root.Children[2]) {
		t.Fatal("what the preview opened should close again when it moves on")
	}
}

func TestIncrementalSearchFromTheCursor(t *testing.T) {
	m := press(testModel(t), "/ q t y enter / q t")
	if got := m.Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("cursor at %s, want the match after the last one", got)
	}
	m = press(testModel(t), "? q t")
	if got := m.Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("? cursor at %s, want the match before the root, wrapped", got)
	}
}

func TestIncrementalSearchEscRestores(t *testing.T) {
	start := press(testModel(t), "j j / q t y enter k")
	want := fingerprint(start)
	if got := fingerprint(press(start, "/ n o t esc")); got != want {
		t.Fatalf("esc should put everything back\n got %s\nwant %s", got, want)
	}
	if got := fingerprint(press(start, "/ n backspace backspace")); got != want {
		t.Fatalf("backspace out of the prompt should too\n got %s\nwant %s", got, want)
	}
	if m := press(start, "/ n o t esc"); len(m.matches) != 2 {
		t.Fatalf("the last search should survive esc: %d matches", len(m.matches))
	}
}

func TestIncrementalSearchNoMatchStaysPut(t *testing.T) {
	m := press(testModel(t), "/ q t z")
	if m.Cursor().Path() != "." || m.Tree().Expanded(m.Tree().Root.Children[2]) {
		t.Fatalf("no match: cursor at %s", m.Cursor().Path())
	}
	if !strings.Contains(m.View(), "0 matches") {
		t.Fatalf("want 0 matches:\n%s", m.View())
	}
}

func TestFinderListsMatchesAndPreviews(t *testing.T) {
	m := press(testModel(t), "f q t y")
	if !m.finding() || len(m.matches) != 2 {
		t.Fatalf("finding %v, %d matches", m.finding(), len(m.matches))
	}
	if got := m.Cursor().Path(); got != ".items[0].qty" {
		t.Fatalf("cursor at %s, want the first hit", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, ".items[1]") || !strings.Contains(v, "qty: null") {
		t.Fatalf("list should show parent path and row:\n%s", v)
	}
	m = press(m, "down")
	if got := m.Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("down: cursor at %s", got)
	}
	if got := press(m, "ctrl+k").Cursor().Path(); got != ".items[0].qty" {
		t.Fatalf("ctrl+k: cursor at %s", got)
	}
	if got := press(m, "down").Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("down should stop at the last hit: %s", got)
	}
	if got := press(m, "up up up").pick; got != 0 {
		t.Fatalf("up should stop at the first hit: %d", got)
	}
}

// Only the hit on show is opened: stepping through a list does not leave the tree open behind it.
func TestFinderOpensOnlyTheHitOnShow(t *testing.T) {
	root := `{"a":{"id":1},"b":{"id":2},"c":{"id":3}}`
	m := fromJSON(t, root)
	m = press(m, "f i d down")
	tr := m.Tree()
	if tr.Expanded(tr.Root.Children[0]) || !tr.Expanded(tr.Root.Children[1]) || tr.Expanded(tr.Root.Children[2]) {
		t.Fatal("want only b open")
	}
	m = press(m, "enter")
	if got := m.Cursor().Path(); got != ".b.id" {
		t.Fatalf("enter: cursor at %s", got)
	}
	if m.finding() || !strings.Contains(m.View(), "match 2/3") {
		t.Fatalf("enter should close the list:\n%s", m.View())
	}
}

// The list is the same search as /: n and N carry on through it after landing, z/ folds to it.
func TestFinderBecomesTheSearch(t *testing.T) {
	m := press(testModel(t), "f q t y enter n")
	if got := m.Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("n after the finder: %s", got)
	}
	if m = press(testModel(t), "f q t y enter z /"); !strings.Contains(m.status, "showing 2 matches") {
		t.Fatalf("z/ after the finder: status %q", m.status)
	}
}

func TestFinderEscRestores(t *testing.T) {
	start := press(testModel(t), "/ n a m e enter")
	want := fingerprint(start)
	if got := fingerprint(press(start, "f q t y down esc")); got != want {
		t.Fatalf("esc should put everything back\n got %s\nwant %s", got, want)
	}
	if got := fingerprint(press(start, "f enter")); got != want {
		t.Fatalf("enter with nothing typed should change nothing\n got %s\nwant %s", got, want)
	}
}

func TestFinderNoMatches(t *testing.T) {
	m := press(testModel(t), "f z z z")
	if m.Cursor().Path() != "." || !strings.Contains(m.View(), "0 matches") {
		t.Fatalf("cursor at %s:\n%s", m.Cursor().Path(), m.View())
	}
	m = press(m, "enter")
	if m.finding() || m.Cursor().Path() != "." || !strings.Contains(m.status, "pattern not found") {
		t.Fatalf("enter: cursor at %s, status %q", m.Cursor().Path(), m.status)
	}
}

// F is the list of the last search, with the selection on the match the cursor is at, so the
// list can be left and picked up again as vim's :copen.
func TestFinderReopensLastSearch(t *testing.T) {
	if m := press(testModel(t), "F"); m.finding() || m.status != "no previous search" {
		t.Fatalf("F with no search: finding %v, status %q", m.finding(), m.status)
	}
	m := press(testModel(t), "/ q t y enter n F")
	if !m.finding() || m.input != "qty" || m.pick != 1 {
		t.Fatalf("finding %v, input %q, pick %d", m.finding(), m.input, m.pick)
	}
	if got := m.Cursor().Path(); got != ".items[1].qty" {
		t.Fatalf("F moved the cursor to %s", got)
	}
	m = press(testModel(t), `/ Q T Y \ c enter F`)
	if m.input != `QTY\c` || len(m.matches) != 2 {
		t.Fatalf(`F should keep \c: input %q, %d matches`, m.input, len(m.matches))
	}
	if m = press(testModel(t), "/ q t y enter z / g g F"); m.pick != 0 {
		t.Fatalf("off a match, F picks the next: %d", m.pick)
	}
}

// Letters are the pattern while the list is open: nothing falls through to the tree or quits.
func TestFinderKeysAreText(t *testing.T) {
	m := press(testModel(t), "f q j k")
	if m.input != "qjk" {
		t.Fatalf("input %q", m.input)
	}
}

func TestFinderListScrolls(t *testing.T) {
	m := send(bigModel(t), tea.WindowSizeMsg{Width: 40, Height: 24})
	m = press(m, "f n")
	rows := m.listRows()
	for range 30 {
		m = press(m, "ctrl+j")
	}
	if m.pick != 30 || m.pick < m.pickTop || m.pick >= m.pickTop+rows {
		t.Fatalf("pick %d outside [%d,%d)", m.pick, m.pickTop, m.pickTop+rows)
	}
	if m.cursor < m.top || m.cursor >= m.top+m.bodyHeight() {
		t.Fatalf("cursor %d off the tree pane [%d,%d)", m.cursor, m.top, m.top+m.bodyHeight())
	}
	if m = press(m, "pgdown"); m.pick != 30+rows {
		t.Fatalf("pgdown: pick %d", m.pick)
	}
	if m = press(m, "pgup pgup pgup pgup pgup"); m.pick != 0 || m.pickTop != 0 {
		t.Fatalf("pgup: pick %d top %d", m.pick, m.pickTop)
	}
}

func TestFinderViewFits(t *testing.T) {
	for _, w := range []int{20, 40, 120} {
		for _, h := range []int{3, 5, 8, 24} {
			m := send(press(testModel(t), "f a"), tea.WindowSizeMsg{Width: w, Height: h})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d: %d lines:\n%s", w, h, len(lines), m.View())
			}
			for i, l := range lines {
				if ansi.StringWidth(l) > w {
					t.Errorf("%dx%d line %d too wide (%d): %q", w, h, i, ansi.StringWidth(l), l)
				}
			}
		}
	}
}

// The selected hit lines up with the rest: its value starts in the same column.
func TestFinderColumnsLineUp(t *testing.T) {
	m := press(testModel(t), "f q t y")
	var cols []int
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if i := strings.Index(l, "qty:"); i >= 0 && strings.HasPrefix(l, ".items") {
			cols = append(cols, i)
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Fatalf("hit columns %v:\n%s", cols, ansi.Strip(m.View()))
	}
}

// The owner's flow on the FIX dictionary: every place a message is named, narrowing as it is typed.
func TestFinderOnFIXDictionary(t *testing.T) {
	m := fixture(t, "FIX4.4.json")
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "f")
	m = type_(m, "tradecapture")
	if len(m.matches) == 0 || m.Cursor() != m.matches[0] {
		t.Fatalf("%d matches, cursor at %s", len(m.matches), m.Cursor().Path())
	}
	all := len(m.matches)
	m = type_(m, "reportack")
	if len(m.matches) == 0 || len(m.matches) >= all {
		t.Fatalf("narrowing went from %d to %d", all, len(m.matches))
	}
}
