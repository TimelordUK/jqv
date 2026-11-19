package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c := Parse("config.toml", `
[set]
number = true
depth  = 2
relativenumber = false
`)
	want := []Setting{{"number", true}, {"depth", int64(2)}, {"relativenumber", false}}
	if !reflect.DeepEqual(c.Set, want) || len(c.Problems) != 0 {
		t.Fatalf("set %v, problems %v", c.Set, c.Problems)
	}
}

func TestParseRepl(t *testing.T) {
	c := Parse("config.toml", "[set]\nnumber = true\n[repl]\npipe = \"bat -l json\"\n")
	if want := []Setting{{"pipe", "bat -l json"}}; !reflect.DeepEqual(c.Repl, want) || len(c.Set) != 1 || len(c.Problems) != 0 {
		t.Fatalf("repl %v, set %v, problems %v", c.Repl, c.Set, c.Problems)
	}
}

func TestParseProblems(t *testing.T) {
	cases := []struct{ text, problem string }{
		{"[keys]\nx = 1", `unknown table [keys]`},
		{"colour = 1", `unknown key "colour" (options go in [set])`},
		{"set = 1", `unknown key "set" (options go in [set])`},
		{"[set\nnumber = true", "toml:"}, // the parser's own message, with its line
	}
	for _, c := range cases {
		got := Parse("config.toml", c.text)
		if len(got.Problems) != 1 || !strings.Contains(got.Problems[0], c.problem) {
			t.Errorf("%q: problems %q, want %q", c.text, got.Problems, c.problem)
		}
	}
	// a bad table does not cost the good one
	c := Parse("config.toml", "[keys]\nx = 1\n[set]\nnumber = true")
	if len(c.Set) != 1 || len(c.Problems) != 1 {
		t.Fatalf("set %v, problems %v", c.Set, c.Problems)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if c := Load(filepath.Join(dir, "none.toml")); len(c.Set) != 0 || len(c.Problems) != 0 {
		t.Fatalf("a missing file is an empty config, not a problem: %+v", c)
	}
	p := filepath.Join(dir, "config.toml")
	os.WriteFile(p, []byte("[set]\nnumber = true\n"), 0o644)
	if c := Load(p); len(c.Set) != 1 || c.Path != p {
		t.Fatalf("%+v", c)
	}
}

func TestPath(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join("x", "my.toml"))
	if got := Path(); got != filepath.Join("x", "my.toml") {
		t.Fatalf("JQV_CONFIG: %s", got)
	}
	t.Setenv("JQV_CONFIG", "")
	if got := Path(); !strings.HasSuffix(got, filepath.Join("jqv", "config.toml")) {
		t.Fatalf("default: %s", got)
	}
}

// The template `jqv config init` writes reads back with no problems, every option in it.
func TestTemplate(t *testing.T) {
	c := Parse("config.toml", Template)
	if len(c.Problems) != 0 || len(c.Set) == 0 || len(c.Repl) == 0 {
		t.Fatalf("set %v, repl %v, problems %v", c.Set, c.Repl, c.Problems)
	}
}

// Init writes the template for a new machine; a config already there is kept unless forced,
// and then it is backed up first, never replaced silently.
func TestInit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "jqv", "config.toml") // the directory is made too
	if backup, err := Init(p, false); err != nil || backup != "" {
		t.Fatalf("a new file: backup %q, err %v", backup, err)
	}
	if b, _ := os.ReadFile(p); string(b) != Template {
		t.Fatalf("wrote %q", b)
	}
	os.WriteFile(p, []byte("[set]\nnumber = false\n"), 0o644)
	if _, err := Init(p, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("an existing file without force: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "[set]\nnumber = false\n" {
		t.Fatalf("an existing file was changed: %q", b)
	}
	backup, err := Init(p, true)
	if err != nil || backup != p+".bak" {
		t.Fatalf("forced: backup %q, err %v", backup, err)
	}
	if b, _ := os.ReadFile(backup); string(b) != "[set]\nnumber = false\n" {
		t.Fatalf("the backup holds %q", b)
	}
	if b, _ := os.ReadFile(p); string(b) != Template {
		t.Fatalf("forced: wrote %q", b)
	}
	// a backup already there is kept too
	if backup, _ := Init(p, true); backup != p+".bak.1" {
		t.Fatalf("the second backup: %q", backup)
	}
}
