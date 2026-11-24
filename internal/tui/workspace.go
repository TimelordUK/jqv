package tui

import (
	"context"
	"sync"
	"time"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/pick"
	"github.com/TimelordUK/jqv/internal/query"
	"github.com/TimelordUK/jqv/internal/shape"
	tea "github.com/charmbracelet/bubbletea"
)

// Workspace is the one owner of the workbench's state (DESIGN §25 rule 3): the documents, the
// program and its latest result. Panes draw from it and ask it for changes; a pane never
// changes another pane.
type Workspace struct {
	Docs  []*Document // a list from the start; one until several are opened (§25)
	Focus int         // the document that is `.`; every other would be $name

	program string
	stages  pick.Pipeline // the program read back as stages (DESIGN §28); never kept apart from it
	at      int           // the stage cursor (§28.2): an index into stages, len(stages) the empty slot after the last
	result  Run           // what the cursor's stage gives: the table
	input   Run           // what goes into it, when that isn't the document
	inRoot  *doc.Node     // input as a tree for the tree pane; nil while the input is the document
	undo    []string      // the programs before this one, newest last

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

	orderOnce sync.Once
	order     *shape.Order
}

// Session is the engine over d, made when first wanted. Safe to call from a run's goroutine.
func (d *Document) Session() *bench.Session {
	d.once.Do(func() { d.session = bench.New(d.Root.Value()) })
	return d.session
}

// Order is d's keys in the order they were written, for showing results with them (Q-010).
// Safe to call from a run's goroutine.
func (d *Document) Order() *shape.Order {
	d.orderOnce.Do(func() { d.order = shape.KeyOrder(d.Root) })
	return d.order
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

// ranMsg brings a run's results back to Update: what the cursor's stage gives, and what goes
// into it with that as a tree, when it isn't the document.
type ranMsg struct {
	gen    int
	out    Run
	in     Run
	inRoot *doc.Node
}

// NewWorkspace holds one document.
func NewWorkspace(name string, root *doc.Node) *Workspace {
	return &Workspace{Docs: []*Document{{Name: name, Root: root}}}
}

// Doc is the document in focus.
func (w *Workspace) Doc() *Document { return w.Docs[w.Focus] }

// Program is the program on show; "" when there is none.
func (w *Workspace) Program() string { return w.program }

// Stages is the program as stages, and At the stage cursor: an index into them, or
// len(Stages()) for the empty slot after the last (DESIGN §28.2).
func (w *Workspace) Stages() pick.Pipeline { return w.stages }
func (w *Workspace) At() int               { return w.at }

// Form is the program's picks, and whether space can change them: only when the program
// starts with picks, or is empty (§25 rule 1).
func (w *Workspace) Form() (pick.Form, bool) { return w.stages.Picks() }

// Columns are the picked columns' names, in pick order, for the table.
func (w *Workspace) Columns() []string {
	f, _ := w.stages.Picks()
	var cs []string
	for _, c := range f.Columns {
		cs = append(cs, c.Name)
	}
	return cs
}

// Result is what the cursor's stage gives; while a newer run is out, Running says so.
func (w *Workspace) Result() Run { return w.result }

// Input is what goes into the cursor's stage as a tree, and its run; nil when that is the
// document, at the first stage or with no program. While a new input is out, the last stays.
func (w *Workspace) Input() (*doc.Node, Run) {
	if w.at == 0 {
		return nil, Run{}
	}
	return w.inRoot, w.input
}

// SetProgram makes p the program, as typed rather than gestured, with the cursor on its last
// stage so the table shows all of it, and gives the command that runs it.
func (w *Workspace) SetProgram(p string) tea.Cmd {
	st := pick.Parse(p)
	return w.set(st, max(0, len(st)-1))
}

// SetPicks writes f as the program's picks (space, Q-013), keeping the stages after them.
func (w *Workspace) SetPicks(f pick.Form) (tea.Cmd, error) {
	st, err := w.stages.WithPicks(f)
	if err != nil {
		return nil, err
	}
	at := w.at
	if len(w.stages) == 0 {
		at = 0 // the first pick: the cursor on the picks, the tree still the document
	}
	return w.set(st, at), nil
}

// Where adds cond at the stage cursor (`=`, DESIGN §28.3): into the filter there, joined by
// and, or with or set by or (`|`, §29); or as a new filter on the end, the cursor staying on
// the empty slot after it.
func (w *Workspace) Where(cond string, or bool) (tea.Cmd, error) {
	st, err := w.stages.Where(w.at, cond, or)
	if err != nil {
		return nil, err
	}
	at := w.at
	if at == len(w.stages) {
		at = len(st)
	}
	return w.set(st, at), nil
}

// Delete takes away the stage under the cursor (D, R-014); the cursor stays where it was,
// now on the stage after, or the empty slot at the end.
func (w *Workspace) Delete() (tea.Cmd, error) {
	st, err := w.stages.Delete(w.at)
	if err != nil {
		return nil, err
	}
	return w.set(st, min(w.at, len(st))), nil
}

// Move moves the stage cursor by d, to the first stage at the least and the empty slot after
// the last at the most. Moving down past a stage is locking it (§28.2).
func (w *Workspace) Move(d int) tea.Cmd {
	at := max(0, min(len(w.stages), w.at+d))
	if at == w.at || len(w.stages) == 0 {
		return nil
	}
	w.at = at
	return w.run()
}

// Undo goes back to the program before the last change; ok is false when there is none.
func (w *Workspace) Undo() (tea.Cmd, bool) {
	if len(w.undo) == 0 {
		return nil, false
	}
	p := w.undo[len(w.undo)-1]
	w.undo = w.undo[:len(w.undo)-1]
	st := pick.Parse(p)
	w.program, w.stages, w.at = p, st, min(w.at, len(st))
	return w.run(), true
}

// set makes st the program with the cursor at at, remembering the program before for undo.
func (w *Workspace) set(st pick.Pipeline, at int) tea.Cmd {
	p := st.String()
	if p != w.program {
		w.undo = append(w.undo, w.program)
	}
	w.program, w.stages, w.at = p, st, at
	return w.run()
}

// run asks for the cursor stage's output and, past the first stage, its input, off the
// keystroke path. The run before, if still going, is cancelled. No program runs nothing.
func (w *Workspace) run() tea.Cmd {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.gen++
	if len(w.stages) == 0 {
		w.result, w.input, w.inRoot = Run{}, Run{}, nil
		return nil
	}
	out := w.stages.Upto(min(w.at+1, len(w.stages)))
	in := w.stages.Upto(w.at) // "" at the first stage: the document
	w.result.Running = true
	if in != "" {
		w.input.Running = true
	}
	gen, d, cols := w.gen, w.Doc(), w.Columns()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	return func() tea.Msg {
		one := func(p string) Run {
			start := time.Now()
			r := d.Session().Run(ctx, p, runOptions)
			return Run{Program: p, Result: r, Took: time.Since(start)}
		}
		msg := ranMsg{gen: gen, out: one(out)}
		switch in {
		case "":
			return msg
		case out:
			msg.in = msg.out
		default:
			msg.in = one(in)
		}
		if msg.in.Err == nil {
			order := d.Order().With([][]string{cols})
			msg.inRoot = doc.FromValues(msg.in.Values, order.Keys)
		}
		return msg
	}
}

// ran takes a run's results, unless a newer program has been asked for since; it says which.
func (w *Workspace) ran(m ranMsg) bool {
	if m.gen != w.gen {
		return false
	}
	w.result = m.out
	if m.in.Program != "" {
		w.input = m.in
		if m.inRoot != nil {
			w.inRoot = m.inRoot
		}
	}
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	return true
}
