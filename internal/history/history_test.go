package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clock gives each call the next second, so records are in a known order.
func clock() func() time.Time {
	t := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Second); return t }
}

func TestRecall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if lines, err := Recall(path, "a.json", 100); err != nil || len(lines) != 0 {
		t.Fatalf("no file is no history: %q %v", lines, err)
	}
	now := clock()
	a, b := Open(path, "a.json"), Open(path, "b.json")
	a.now, b.now = now, now
	for _, s := range []struct {
		l      *Log
		line   string
		failed bool
	}{
		{a, ".a1", false},
		{b, ".b1", false},
		{a, ".broken[", true},
		{a, ".a2", false},
		{b, ".b2", false},
		{a, ".a1", false}, // again: once, as the newest
	} {
		if err := s.l.Add(s.line, s.failed, time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := Recall(path, "a.json", 100)
	if err != nil {
		t.Fatal(err)
	}
	// oldest first; up-arrow walks from the end: this document's newest, then the others
	if got := strings.Join(lines, " "); got != ".b1 .b2 .a2 .a1" {
		t.Errorf("Recall = %q", got)
	}
	if lines, _ := Recall(path, "a.json", 2); strings.Join(lines, " ") != ".b2 .a1" {
		t.Errorf("max keeps the newest: %q", lines)
	}
	b2, _ := os.ReadFile(path)
	if !strings.Contains(string(b2), `"sh":"jqv"`) || !strings.Contains(string(b2), `"exit":1`) {
		t.Errorf("records in hit's format, the broken line marked:\n%s", b2)
	}
}

func TestPath(t *testing.T) {
	t.Setenv("JQV_HISTORY", "x.jsonl")
	if Path() != "x.jsonl" {
		t.Errorf("JQV_HISTORY: %q", Path())
	}
}

func TestNoPathRecordsNothing(t *testing.T) {
	var l *Log
	if err := l.Add(".a", false, 0); err != nil {
		t.Error(err)
	}
	if err := Open("", "a.json").Add(".a", false, 0); err != nil {
		t.Error(err)
	}
}
