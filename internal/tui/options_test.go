package tui

import (
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/config"
)

// typed sends a : command line, one rune at a time, as the terminal would.
func typed(m Model, line string) Model {
	m = send(m, key(":"))
	for _, r := range line {
		m = send(m, key(string(r)))
	}
	return send(m, key("enter"))
}

func TestSetForms(t *testing.T) {
	cases := []struct {
		line   string
		check  func(Model) bool
		status string
	}{
		{"set nu", func(m Model) bool { return m.number }, ""},
		{"set number", func(m Model) bool { return m.number }, ""},
		{"se nu", func(m Model) bool { return m.number }, ""},
		{"set nu!", func(m Model) bool { return m.number }, ""},
		{"set invnu", func(m Model) bool { return m.number }, ""},
		{"set nu rnu", func(m Model) bool { return m.number && m.relnum }, ""},
		{"set nu?", func(m Model) bool { return !m.number }, "nonumber"},
		{"set scrolllock", func(m Model) bool { return m.lock }, ""},
		{"set depth=2", func(m Model) bool { return m.Tree().Expanded(m.Tree().Root.Children[1]) }, ""},
		{"set depth", func(m Model) bool { return true }, "depth=1"},
		{"set depth?", func(m Model) bool { return true }, "depth=1"},
		{"set depth=0", func(m Model) bool { return m.depth == 1 }, "depth is 1 or more, not 0"},
		{"set depth=x", func(m Model) bool { return m.depth == 1 }, `depth is a whole number, not "x"`},
		{"set nodepth", func(m Model) bool { return true }, "depth is a number, not on or off"},
		{"set nu=3", func(m Model) bool { return !m.number }, "number is on or off: number, nonumber"},
		{"set numbr", func(m Model) bool { return true }, "unknown option: numbr"},
		{"set nu numbr", func(m Model) bool { return m.number }, "unknown option: numbr"},
		{"set", func(m Model) bool { return true }, "all options at their defaults"},
		{"set all", func(m Model) bool { return true }, "nonumber  norelativenumber  noscrolllock  depth=1"},
	}
	for _, c := range cases {
		m := typed(testModel(t), c.line)
		if !c.check(m) || m.status != c.status {
			t.Errorf(":%s: check %v, status %q, want %q", c.line, c.check(m), m.status, c.status)
		}
	}
	m := typed(typed(testModel(t), "set nu depth=2"), "set") // bare :set lists what differs
	if m.status != "number  depth=2" {
		t.Errorf(":set after changes: %q", m.status)
	}
	m = typed(typed(testModel(t), "set nu"), "set nu!")
	if m.number {
		t.Error(":set nu! should toggle back off")
	}
	m = typed(typed(testModel(t), "set nu rnu"), "set nonu") // as before T-025: nonu clears the gutter
	if m.number || m.relnum {
		t.Error(":set nonu should turn the whole gutter off")
	}
	m = press(typed(testModel(t), "set scrolllock"), "z l")
	if m.lock {
		t.Error("zl and scrolllock are the same switch")
	}
}

func TestConfigure(t *testing.T) {
	m := testModel(t)
	m.Configure(config.Config{Path: `C:\x\config.toml`, Set: []config.Setting{
		{Name: "number", Value: true},
		{Name: "depth", Value: int64(2)},
		{Name: "nu", Value: true},              // short names are for typing, not files
		{Name: "relativenumber", Value: "yes"}, // wrong type
		{Name: "scrolllock", Value: int64(1)},  // wrong type
		{Name: "depth", Value: float64(2.5)},   // wrong type
		{Name: "numbr", Value: true},           // typo
	}, Problems: []string{"unknown table [keys]"}})
	if !m.number || m.depth != 2 || m.relnum || m.lock {
		t.Fatalf("number %v depth %d relnum %v lock %v", m.number, m.depth, m.relnum, m.lock)
	}
	for _, want := range []string{
		"config.toml: ", "unknown table [keys]", `unknown option "nu" (write number)`,
		`relativenumber is true or false, not "yes"`, "scrolllock is true or false, not 1",
		"depth is a whole number, not 2.5", `unknown option "numbr"`,
	} {
		if !strings.Contains(m.status, want) {
			t.Errorf("status %q lacks %q", m.status, want)
		}
	}
	if m = press(m, "j"); m.status != "" {
		t.Error("the config report is one-shot, like any status")
	}

	m = testModel(t)
	m.Configure(config.Config{Path: "config.toml"})
	if m.status != "" {
		t.Errorf("a clean config says nothing: %q", m.status)
	}
}

func TestHelpListsOptions(t *testing.T) {
	m := testModel(t)
	m.Configure(config.Config{Path: `C:\x\jqv\config.toml`})
	text := strings.Join(m.helpLines(), "\n")
	for _, o := range options {
		if !strings.Contains(text, o.name) || !strings.Contains(text, o.what) {
			t.Errorf("help lacks option %s", o.name)
		}
	}
	if !strings.Contains(text, `C:\x\jqv\config.toml`) {
		t.Error("help should say where the config file is")
	}
}
