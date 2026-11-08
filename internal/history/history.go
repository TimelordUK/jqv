// Package history is the REPL's query history (R-001, DESIGN §18), kept in hit's format with
// hit's own packages (github.com/TimelordUK/hit/history): an append-only JSONL file owned by
// jqv, one cmd record per line typed and an end record with how it went. cmd is the line,
// cwd the document's full path, sh "jqv", exit 0 or 1. Broken lines are kept, marked, and
// hidden when recalling, so nothing typed is lost and nothing broken comes back first.
package history

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/TimelordUK/hit/history/record"
	"github.com/TimelordUK/hit/history/search"
	"github.com/TimelordUK/hit/history/store"
)

// Shell is the sh field of jqv's records.
const Shell = "jqv"

// Path is where the history lives: JQV_HISTORY, else %LOCALAPPDATA%\jqv\history.jsonl on
// Windows and $XDG_STATE_HOME/jqv (~/.local/state/jqv) elsewhere. "" when there is no home.
func Path() string {
	if p := os.Getenv("JQV_HISTORY"); p != "" {
		return p
	}
	var dir string
	switch {
	case runtime.GOOS == "windows":
		dir = os.Getenv("LOCALAPPDATA")
	case os.Getenv("XDG_STATE_HOME") != "":
		dir = os.Getenv("XDG_STATE_HOME")
	default:
		if h, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(h, ".local", "state")
		}
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "jqv", "history.jsonl")
}

// Log appends one REPL session's lines.
type Log struct {
	path, doc, session, host string
	now                      func() time.Time
}

// Open is a session's log, appending to path, for the document at doc (made absolute).
func Open(path, doc string) *Log {
	if abs, err := filepath.Abs(doc); err == nil {
		doc = abs
	}
	host, _ := os.Hostname()
	l := &Log{path: path, doc: doc, host: host, now: time.Now}
	l.session = record.NewID(l.now(), rand.Reader)
	return l
}

// Add records a line typed and how it went.
func (l *Log) Add(line string, failed bool, took time.Duration) error {
	if l == nil || l.path == "" || line == "" {
		return nil
	}
	t := l.now()
	exit, ms := 0, took.Milliseconds()
	if failed {
		exit = 1
	}
	id := record.NewID(t, rand.Reader)
	ts := record.FormatTime(t)
	return store.Append(l.path,
		&record.Record{K: record.KindCmd, ID: id, TS: ts, Cmd: line, Cwd: l.doc, Sh: Shell, Host: l.host, Sid: l.session},
		&record.Record{K: record.KindEnd, ID: id, TS: ts, Exit: &exit, Ms: &ms},
	)
}

// Recall is the lines for up-arrow, oldest first, at most max: the ones that ran, each once,
// most recent last, with this document's after the others so up finds them first. A file
// that isn't there is an empty history.
func Recall(path, doc string, max int) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	recs, _, err := store.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if abs, err := filepath.Abs(doc); err == nil {
		doc = abs
	}
	rs := search.Search(store.Build(recs), search.Query{
		Scope: search.ScopeAll, Sort: search.SortRecent, Shell: Shell, Cwd: doc,
		HideFailed: true, Limit: max,
	})
	var here, other []string // newest first, as Search gives them
	for _, r := range rs {
		if r.InCwd {
			here = append(here, r.Entry.Cmd)
		} else {
			other = append(other, r.Entry.Cmd)
		}
	}
	lines := make([]string, 0, len(rs))
	for i := len(other) - 1; i >= 0; i-- {
		lines = append(lines, other[i])
	}
	for i := len(here) - 1; i >= 0; i-- {
		lines = append(lines, here[i])
	}
	return lines, nil
}
