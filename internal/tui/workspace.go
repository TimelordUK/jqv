package tui

import (
	"context"
	"sync"
	"time"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// Workspace is the one owner of the workbench's state (DESIGN §25 rule 3): the documents, the
// program and its latest result. Panes draw from it and ask it for changes; a pane never
// changes another pane.
type Workspace struct {
	Docs  []*Document // a list from the start; one until several are opened (§25)
	Focus int         // the document that is `.`; every other would be $name

	program string
	result  Run

	gen    int                // the latest run asked for; a result from an older one is dropped
	cancel context.CancelFunc // stops the run in flight, if any
}

// Document is a loaded value: its tree for the panes, and, made on the first run and kept,
// the session that runs jq on it (GRID §10: Value() costs about as much memory again as the
// tree, so a document only looked at never pays it).
type Document struct {
	Name string // the file name, or "stdin"
	Root *doc.Node

	once    sync.Once
	session *bench.Session
}

// Session is the engine over d, made when first wanted. Safe to call from a run's goroutine.
func (d *Document) Session() *bench.Session {
	d.once.Do(func() { d.session = bench.New(d.Root.Value()) })
	return d.session
}

// Run is a program's result as the panes see it.
type Run struct {
	Program string
	query.Result
	Took    time.Duration
	Running bool // asked for, not back yet; Result is the previous program's meanwhile
}

// runOptions bound the TUI's runs. A table wants every row, so the cap is far above the
// REPL's 50 and is there only so an endless generator can't fill memory.
var runOptions = query.Options{Timeout: 10 * time.Second, Max: 1_000_000}

// ranMsg brings a run's result back to Update.
type ranMsg struct {
	gen int
	run Run
}

// NewWorkspace holds one document.
func NewWorkspace(name string, root *doc.Node) *Workspace {
	return &Workspace{Docs: []*Document{{Name: name, Root: root}}}
}

// Doc is the document in focus.
func (w *Workspace) Doc() *Document { return w.Docs[w.Focus] }

// Program is the program on show; "" when there is none.
func (w *Workspace) Program() string { return w.program }

// Result is the latest result; while a newer run is out, Running says so.
func (w *Workspace) Result() Run { return w.result }

// SetProgram makes p the program and gives the command that runs it, off the keystroke path.
// The run before it, if still going, is cancelled. An empty program runs nothing.
func (w *Workspace) SetProgram(p string) tea.Cmd {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.gen++
	w.program = p
	if p == "" {
		w.result = Run{}
		return nil
	}
	w.result.Running = true
	gen, d := w.gen, w.Doc()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	return func() tea.Msg {
		start := time.Now()
		r := d.Session().Run(ctx, p, runOptions)
		return ranMsg{gen: gen, run: Run{Program: p, Result: r, Took: time.Since(start)}}
	}
}

// ran takes a run's result, unless a newer program has been asked for since; it says which.
func (w *Workspace) ran(m ranMsg) bool {
	if m.gen != w.gen {
		return false
	}
	w.result = m.run
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	return true
}
