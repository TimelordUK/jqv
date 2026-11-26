package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// count is how many runs have field k equal to v.
func count(m Model, k, v string) int {
	n := 0
	for _, r := range m.ws.Doc().Root.Children {
		for _, c := range r.Children {
			if c.Key == k && c.Raw == v {
				n++
			}
		}
	}
	return n
}

// DESIGN §28: J locks the picks and the tree shows what they give; = on a value there adds a
// filter on the end; K steps back, the tree showing what goes into the filter and = adding to
// it; u undoes; K to the first stage brings the document back as it was left.
func TestStagesLockFilterStepBack(t *testing.T) {
	m := do(onRecord(t), "j space j j j space") // Job, Status
	if m.ws.Program() != ".[] | {Job, Status}" || m.ws.At() != 0 || !m.onDocument() {
		t.Fatalf("picks: program %q at %d, tree on the document %v", m.ws.Program(), m.ws.At(), m.onDocument())
	}
	runs := len(m.ws.Doc().Root.Children)

	m = do(m, "J")
	if m.ws.At() != 1 || m.onDocument() || len(m.tree.Root.Children) != runs {
		t.Fatalf("J: at %d, tree on the document %v with %d values, want the %d rows",
			m.ws.At(), m.onDocument(), len(m.tree.Root.Children), runs)
	}
	if !strings.Contains(m.View(), "the end of 1 stage:") {
		t.Error("the header doesn't say the cursor is at the end")
	}
	if m = do(m, "j j space"); !strings.Contains(m.status, "K back to the picks") {
		t.Errorf("space off the document: status %q", m.status)
	}

	m = do(m, "j") // [0].Status in the rows
	n := m.Cursor()
	if n.Key != "Status" {
		t.Fatalf("cursor on %q", n.Key)
	}
	status := n.Raw
	m = do(m, "=")
	want := fmt.Sprintf(`.[] | {Job, Status} | select(.Status == %q)`, status)
	if m.ws.Program() != want || m.ws.At() != 2 {
		t.Fatalf("=: program %q at %d, want %q at the end", m.ws.Program(), m.ws.At(), want)
	}
	matching := count(m, "Status", status)
	if m.table.rows() != matching || len(m.tree.Root.Children) != matching {
		t.Errorf("filtered: table %d rows, tree %d, want %d", m.table.rows(), len(m.tree.Root.Children), matching)
	}

	m = do(m, "K")
	if m.ws.At() != 1 || len(m.tree.Root.Children) != runs || m.table.rows() != matching {
		t.Fatalf("K onto the filter: at %d, tree %d, table %d", m.ws.At(), len(m.tree.Root.Children), m.table.rows())
	}
	m = do(m, "k =") // [0].Job, added to the filter
	if !strings.Contains(m.ws.Program(), `select(.Status == `+fmt.Sprintf("%q", status)+` and .Job == `) {
		t.Fatalf("= on the filter: %q", m.ws.Program())
	}
	m = do(m, "u")
	if m.ws.Program() != want {
		t.Fatalf("u: %q, want %q", m.ws.Program(), want)
	}

	m = do(m, "K")
	if !m.onDocument() || m.Cursor().Path() != ".[3].Status" {
		t.Errorf("K to the picks: on the document %v, cursor at %s", m.onDocument(), m.Cursor().Path())
	}
	m = do(m, "k space") // Number, picked on the picks stage: the filter after it stays
	if got := fmt.Sprintf(`.[] | {Job, Status, Number} | select(.Status == %q)`, status); m.ws.Program() != got {
		t.Errorf("a pick under a filter: %q, want %q", m.ws.Program(), got)
	}
}

// = on the document waits for inserting (R-015); J and K with nothing picked say why.
func TestStagesRefusals(t *testing.T) {
	m := do(onRecord(t), "=")
	if m.ws.Program() != "" || !strings.Contains(m.status, "pick fields first") {
		t.Errorf("= with no program: %q, status %q", m.ws.Program(), m.status)
	}
	if m = do(m, "J"); !strings.Contains(m.status, "no stages yet") {
		t.Errorf("J with no program: status %q", m.status)
	}
	m = do(m, "j space j =")
	if !strings.Contains(m.status, "R-015") {
		t.Errorf("= on the document: status %q", m.status)
	}
}

func open(t *testing.T, name string) Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return send(New(name, root), tea.WindowSizeMsg{Width: 140, Height: 30})
}

// DESIGN §29, the owner's orgchart case: pick a manager's id and name, then x on
// direct_reports gives a row per report with the manager's fields on it, the array marked
// in every record. Locked, = and | filter the rows with or.
func TestStagesJoinThenOr(t *testing.T) {
	m := do(landOn(t, open(t, "orgchart.json"), ".[1].id"), "space")
	m = do(landOn(t, m, ".[1].name"), "space")
	m = do(landOn(t, m, ".[1].direct_reports"), "x")
	if got, want := m.ws.Program(), ".[] | {id, name, direct_reports: .direct_reports[]?}"; got != want {
		t.Fatalf("program %q, want %q (status %q)", got, want, m.status)
	}
	reports := query.Run("[.[].direct_reports[]] | length", m.ws.Doc().Root.Value(), nil, query.Options{})
	if n := m.table.rows(); float64(n) != toFloat(reports.Values[0]) {
		t.Errorf("%d rows, want one per report (%v)", n, reports.Values[0])
	}
	if m = landOn(t, m, ".[3].direct_reports"); !m.joined(m.Cursor()) || !strings.Contains(m.View(), "● direct_reports") {
		t.Error("direct_reports in another record isn't marked joined")
	}

	m = do(m, "J j j j j") // the end; the first row's report
	if n := m.Cursor(); n.Key != "direct_reports" || n.Raw != "E0002" {
		t.Fatalf("cursor on %s %q", n.Key, n.Raw)
	}
	m = do(m, "= K j j j j |") // a filter, then onto it and the second row's report with or
	want := `.[] | {id, name, direct_reports: .direct_reports[]?} | select(.direct_reports == "E0002" or .direct_reports == "E0003")`
	if m.ws.Program() != want {
		t.Fatalf("program %q, want %q", m.ws.Program(), want)
	}
	if m.table.rows() != 2 {
		t.Errorf("%d rows, want E0002 and E0003", m.table.rows())
	}
}

// From daily use (TeamCity, wanting Agent == X or Agent == Y): at the end the tree only shows
// rows that passed, so = again on an Agent wrote the same filter twice, and | started a new
// stage. Both are refused with where to go instead; D deletes the stage under the cursor.
func TestStagesRepeatOrAndDelete(t *testing.T) {
	m := do(landOn(t, teamcity(t), ".[3].Agent"), "space J j j =")
	once := m.ws.Program()
	if !strings.Contains(once, "| select(.Agent == ") {
		t.Fatalf("program %q", once)
	}
	if m = do(m, "="); m.ws.Program() != once || !strings.Contains(m.status, "already") {
		t.Errorf("= again: %q, status %q", m.ws.Program(), m.status)
	}
	if m = do(m, "|"); m.ws.Program() != once || !strings.Contains(m.status, "K onto") {
		t.Errorf("| on the end: %q, status %q", m.ws.Program(), m.status)
	}
	m = do(m, "K D")
	if m.ws.Program() != ".[] | {Agent}" || m.ws.At() != 1 {
		t.Errorf("D: %q at %d, want the picks with the cursor at the end", m.ws.Program(), m.ws.At())
	}
	if m = do(m, "D"); !strings.Contains(m.status, "K onto") {
		t.Errorf("D on the end: status %q", m.status)
	}
	if m = do(m, "u"); m.ws.Program() != once {
		t.Errorf("u after D: %q", m.ws.Program())
	}
}

// The nobel prizes (owner 2026-11-24): year and category picked, x on laureates joins an
// array of records empty, a row per laureate; space on a laureate's field puts it beside the
// prize's, and laureates is marked joined in every prize (DESIGN §29.3).
func TestStagesJoinRecords(t *testing.T) {
	m := do(landOn(t, open(t, "nobel_prize.json"), ".prizes[0].year"), "space")
	m = do(landOn(t, m, ".prizes[0].category"), "space")
	m = do(landOn(t, m, ".prizes[0].laureates"), "x")
	if got, want := m.ws.Program(), ".prizes[] | {year, category} + (.laureates[]? | {})"; got != want {
		t.Fatalf("program %q, want %q (status %q)", got, want, m.status)
	}
	m = do(landOn(t, m, ".prizes[0].laureates[0].surname"), "space")
	if got, want := m.ws.Program(), ".prizes[] | {year, category} + (.laureates[]? | {surname})"; got != want {
		t.Fatalf("program %q, want %q (status %q)", got, want, m.status)
	}
	laureates := query.Run("[.prizes[].laureates[]?] | length", m.ws.Doc().Root.Value(), nil, query.Options{})
	if n := m.table.rows(); float64(n) != toFloat(laureates.Values[0]) {
		t.Errorf("%d rows, want one per laureate (%v)", n, laureates.Values[0])
	}
	if m = landOn(t, m, ".prizes[2].laureates"); !m.joined(m.Cursor()) || !strings.Contains(m.View(), "● laureates") {
		t.Error("laureates in another prize isn't marked joined")
	}
	if m = landOn(t, m, ".prizes[2].laureates[0].surname"); !m.picked(m.Cursor()) {
		t.Error("surname in another laureate isn't marked picked")
	}
	m = do(landOn(t, m, ".prizes[1].laureates"), "x")
	if got := m.ws.Program(); got != ".prizes[] | {year, category}" {
		t.Errorf("x again: %q", got)
	}
}

// ! is = turned round (Q-071): at the end it leaves out rows with this value; on a filter it
// adds another value to leave out with and; the value's opposite in the same filter is refused.
func TestStagesNotEqual(t *testing.T) {
	m := do(onRecord(t), "j space j j j space J j j j") // picks Job, Status; the end; [0].Status
	runs := len(m.ws.Doc().Root.Children)
	status := m.Cursor().Raw
	m = do(m, "!")
	want := fmt.Sprintf(`.[] | {Job, Status} | select(.Status != %q)`, status)
	if m.ws.Program() != want {
		t.Fatalf("!: %q, want %q (status %q)", m.ws.Program(), want, m.status)
	}
	if n := m.table.rows(); n != runs-count(m, "Status", status) {
		t.Errorf("%d rows, want all but the %s ones", n, status)
	}
	m = do(m, "K k !") // [0].Job going in, left out as well
	if !strings.Contains(m.ws.Program(), fmt.Sprintf(`select(.Status != %q and .Job != `, status)) {
		t.Fatalf("! on the filter: %q", m.ws.Program())
	}
	m = do(m, "u j =") // the same Status, now wanted
	if !strings.Contains(m.status, "!=") || strings.Contains(m.ws.Program(), "==") {
		t.Errorf("== beside its != : program %q, status %q", m.ws.Program(), m.status)
	}
}

// ~ on a filter stage turns it round (Q-073): = on Status and Job, then ~, is every run but
// those; ~ again puts it back.
func TestStagesNotFilter(t *testing.T) {
	m := do(onRecord(t), "j space j j j space J j j j = K k =") // Status == s and Job == j
	both := m.table.rows()
	runs := len(m.ws.Doc().Root.Children)
	m = do(m, "~")
	if !strings.HasSuffix(m.ws.Program(), ") | not)") || m.table.rows() != runs-both {
		t.Fatalf("~: %q, %d rows, want %d", m.ws.Program(), m.table.rows(), runs-both)
	}
	m = do(m, "~")
	if strings.Contains(m.ws.Program(), "not") || m.table.rows() != both {
		t.Errorf("~ again: %q, %d rows", m.ws.Program(), m.table.rows())
	}
	if m = do(m, "J ~"); !strings.Contains(m.status, "K onto a filter") {
		t.Errorf("~ at the end: status %q", m.status)
	}
}

// < and > on a value (Q-072): DurationSec has nulls in TeamCity, so the filter carries the
// number guard; > on another row's value going in makes it a range, the guard written once.
func TestStagesCompare(t *testing.T) {
	m := do(onRecord(t), "j space"+strings.Repeat(" j", 12)+" space") // Job, DurationSec
	if m.ws.Program() != ".[] | {Job, DurationSec}" {
		t.Fatalf("picks: %q", m.ws.Program())
	}
	m = do(m, "J j j j") // [0].DurationSec, 610.0
	if n := m.Cursor(); n.Key != "DurationSec" {
		t.Fatalf("cursor on %s", n.Path())
	}
	m = do(m, "<")
	g := `(.DurationSec | type) == "number" and `
	if want := `.[] | {Job, DurationSec} | select(` + g + `.DurationSec < 610.0)`; m.ws.Program() != want {
		t.Fatalf("<: %q\n want %q (status %q)", m.ws.Program(), want, m.status)
	}
	m = do(m, "K j j j >") // [1].DurationSec going in, 12.0
	want := `.[] | {Job, DurationSec} | select(` + g + `.DurationSec < 610.0 and .DurationSec > 12.0)`
	if m.ws.Program() != want {
		t.Fatalf("range: %q\n want %q", m.ws.Program(), want)
	}
	n := query.Run(`[.[] | select(`+g+`.DurationSec < 610 and .DurationSec > 12)] | length`, m.ws.Doc().Root.Value(), nil, query.Options{})
	if float64(m.table.rows()) != toFloat(n.Values[0]) {
		t.Errorf("%d rows, want %v", m.table.rows(), n.Values[0])
	}
	if m = do(m, "k k <"); m.status == "" || m.ws.Program() != want { // the [1] record itself
		t.Errorf("< on a record: status %q, program %q", m.status, m.ws.Program())
	}
}
