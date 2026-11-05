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
