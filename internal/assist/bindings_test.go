package assist

import (
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/config"
	"github.com/TimelordUK/jqv/internal/editor"
)

func reader() *editor.Reader {
	r := &editor.Reader{Keys: editor.DefaultKeys()}
	Bind(r, bench.New(nil))
	return r
}

// [keys] in config.toml (T-005, owner 2026-11-19): assist's keys are defaults a config can
// change. A value is an action, a command line, or "" for nothing; a mistake is said and
// skipped, as everywhere in the config.
func TestBindKeys(t *testing.T) {
	r := reader()
	if r.Keys["f2"] != ":!" || r.Submits[":!"] == nil {
		t.Fatalf("F2 is the command :! by default: %q", r.Keys["f2"])
	}
	problems := BindKeys(r, []config.Setting{
		{Name: "f6", Value: ":pp"},             // a command
		{Name: "f9", Value: "layout"},          // an action, on another key
		{Name: "f8", Value: ""},                // nothing
		{Name: "ctrl+o", Value: "kill-to-end"}, // an editor action
		{Name: "f2", Value: ":! clip"},         // a default changed
		{Name: "f7", Value: "lay-out"},         // no such action
		{Name: "f99", Value: "help"},           // no such key
		{Name: "enter", Value: "help"},         // the way to run stays
		{Name: "f4", Value: 3},                 // not text
	})
	if got := r.Keys["f6"]; got != ":pp" || r.Submits[got]("x", 0) != ":pp" {
		t.Errorf("f6 runs :pp: %q", got)
	}
	if r.Keys["f9"] != Layout || r.Keys["ctrl+o"] != editor.KillToEnd {
		t.Errorf("actions: f9 %q, ctrl+o %q", r.Keys["f9"], r.Keys["ctrl+o"])
	}
	if _, ok := r.Keys["f8"]; ok {
		t.Errorf("f8 unbound: %q", r.Keys["f8"])
	}
	if r.Submits[r.Keys["f2"]]("", 0) != ":! clip" {
		t.Errorf("f2 changed: %q", r.Keys["f2"])
	}
	want := []string{`[keys] f7: no action "lay-out"`, `[keys] f99: no key`, `[keys] enter: stays`, `[keys] f4: want`}
	if len(problems) != len(want) {
		t.Fatalf("problems %q", problems)
	}
	for i, w := range want {
		if !strings.HasPrefix(problems[i], w) {
			t.Errorf("problem %d: %q, want %q…", i, problems[i], w)
		}
	}
	if r.Keys["enter"] != editor.Submit || r.Keys["f7"] != "" || r.Keys["f4"] != "" {
		t.Errorf("a mistake changes nothing: enter %q, f7 %q, f4 %q", r.Keys["enter"], r.Keys["f7"], r.Keys["f4"])
	}
}

// The template's [keys] lists the defaults, commented, so a new machine sees what can change.
func TestTemplateListsKeys(t *testing.T) {
	for _, d := range Defaults {
		if !strings.Contains(config.Template, "# "+keyLine(d)) {
			t.Errorf("the template doesn't list %q", keyLine(d))
		}
	}
}

// :keys lists what each key does now: assist's and config's first, then the editor's.
func TestKeysText(t *testing.T) {
	r := reader()
	BindKeys(r, []config.Setting{{Name: "f6", Value: ":pp"}, {Name: "f8", Value: ""}})
	got := KeysText(r)
	for _, want := range []string{"f1         help\n", "f2         :!\n", "f6         :pp\n", "ctrl+a     home\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "layout") {
		t.Errorf("f8 unbound, so layout has no key:\n%s", got)
	}
	if strings.Index(got, "f6 ") > strings.Index(got, "ctrl+a ") {
		t.Errorf("jqv's keys come before the editor's:\n%s", got)
	}
}
