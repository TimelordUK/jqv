package tui

import (
	"fmt"
	"strings"
	"testing"
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
