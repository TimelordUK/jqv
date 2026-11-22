package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimelordUK/jqv/internal/doc"
	tea "github.com/charmbracelet/bubbletea"
)

func teamcity(t *testing.T) Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "teamcity_runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := doc.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return send(New("teamcity_runs.json", root), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// run delivers a command's message to the model, as bubbletea would.
func run(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	return send(m, cmd())
}

// A document only looked at never pays for Value() (GRID §10).
func TestWorkspaceLazySession(t *testing.T) {
	m := press(teamcity(t), "j j / S U C enter")
	if m.ws.Doc().session != nil {
		t.Fatal("the session was made without a run")
	}
}

func TestWorkspaceRun(t *testing.T) {
	m := teamcity(t)
	cmd := m.ws.SetProgram(".[] | {Job, Status}")
	if !m.ws.Result().Running {
		t.Error("asked for but not marked running")
	}
	m = run(m, cmd)
	r := m.ws.Result()
	if r.Running || r.Err != nil || r.Program != ".[] | {Job, Status}" {
		t.Fatalf("result: %+v", r)
	}
	n := len(m.ws.Doc().Root.Children)
	if len(r.Values) != n {
		t.Errorf("%d values, want one per run (%d)", len(r.Values), n)
	}
	// The tree is untouched by a run: Read still looks as it did.
	if m.ws.Program() == "" || m.Cursor() != m.Tree().Root {
		t.Error("a run moved the tree")
	}
}

// A result from a program that has since been replaced is dropped, and its run is cancelled.
func TestWorkspaceStale(t *testing.T) {
	m := teamcity(t)
	old := m.ws.SetProgram(".[] | .Job")
	cur := m.ws.SetProgram(".[] | .Status")
	m = run(m, cur)
	stale := old().(ranMsg)
	if !errors.Is(stale.run.Err, context.Canceled) {
		t.Errorf("the replaced run went on: %v", stale.run.Err)
	}
	m = send(m, stale)
	if r := m.ws.Result(); r.Program != ".[] | .Status" || r.Err != nil {
		t.Errorf("a stale result replaced the current one: %+v", r)
	}
}

func TestWorkspaceNoProgram(t *testing.T) {
	m := teamcity(t)
	m = run(m, m.ws.SetProgram(".[0]"))
	if cmd := m.ws.SetProgram(""); cmd != nil {
		t.Error("an empty program should run nothing")
	}
	if r := m.ws.Result(); r.Program != "" || r.Values != nil || r.Running {
		t.Errorf("no program, but a result: %+v", r)
	}
}

func TestWorkspaceError(t *testing.T) {
	m := teamcity(t)
	m = run(m, m.ws.SetProgram(".[] | .Job + 1"))
	if r := m.ws.Result(); r.Err == nil || r.Running {
		t.Errorf("want an error: %+v", r)
	}
}
