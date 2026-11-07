// Package config reads config.toml (DESIGN §13).
//
// The file is a list of :set options under [set], and the REPL's settings under [repl]. This
// package only reads it: what the options are, and whether a value suits one, is the viewer's
// and the REPL's business, since they check the same things for :set. A broken config must never stop jqv, so problems are collected, not returned as errors,
// and whatever could be read is still used.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is what config.toml said, as far as it could be read.
type Config struct {
	Path     string
	Set      []Setting // the [set] table, in file order
	Repl     []Setting // the [repl] table, in file order (DESIGN §18)
	Problems []string  // everything wrong, in file order
}

// Setting is one line of [set]. Value is as TOML gave it: bool, int64, float64, string, ...
type Setting struct {
	Name  string
	Value any
}

// Path is where the config lives: $JQV_CONFIG, or config.toml in the user config directory
// (%APPDATA%\jqv on Windows, ~/.config/jqv elsewhere).
func Path() string {
	if p := os.Getenv("JQV_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "jqv", "config.toml")
}

// Load reads the config at path. A missing file is an empty config, not a problem: most
// people have none.
func Load(path string) Config {
	if path == "" {
		return Config{}
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Path: path}
	}
	if err != nil {
		return Config{Path: path, Problems: []string{err.Error()}}
	}
	return Parse(path, string(b))
}

// Parse reads config text; path is only kept for messages.
func Parse(path, text string) Config {
	c := Config{Path: path}
	var raw map[string]any
	md, err := toml.Decode(text, &raw)
	if err != nil {
		c.Problems = append(c.Problems, err.Error())
		return c
	}
	tables := map[string]*[]Setting{"set": &c.Set, "repl": &c.Repl}
	for _, k := range md.Keys() { // Keys keeps file order; the map does not
		t, _ := raw[k[0]].(map[string]any)
		switch {
		case tables[k[0]] != nil && t != nil && len(k) == 1:
		case tables[k[0]] != nil && t != nil && len(k) == 2:
			*tables[k[0]] = append(*tables[k[0]], Setting{k[1], t[k[1]]})
		case tables[k[0]] != nil && t != nil:
			c.Problems = append(c.Problems, fmt.Sprintf("[%s] %s: a table inside [%s]", k[0], k[1], k[0]))
		case len(k) == 1 && md.Type(k...) == "Hash":
			c.Problems = append(c.Problems, fmt.Sprintf("unknown table [%s]", k[0]))
		case len(k) == 1:
			c.Problems = append(c.Problems, fmt.Sprintf("unknown key %q (options go in [set])", k[0]))
		}
	}
	return c
}
