package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/config"
)

// option is one :set option (DESIGN §13). The same table serves :set, config.toml's [set] and
// the help, so anything one of them can change, the others can too.
type option struct {
	name  string // long name, the one config.toml uses
	short string // vim-style alias for typing, or ""
	what  string // one line for the help
	flag  func(m *Model) *bool
	num   func(m *Model) *int
	least int            // smallest value of a number option
	def   int            // default of a number option; flags default to off
	after func(m *Model) // runs after the value changes
}

var options = []option{
	{name: "number", short: "nu", what: "row numbers in a gutter",
		flag:  func(m *Model) *bool { return &m.number },
		after: func(m *Model) { m.relnum = m.relnum && m.number }}, // nonu clears the gutter, as before T-025
	{name: "relativenumber", short: "rnu", what: "row numbers relative to the cursor",
		flag: func(m *Model) *bool { return &m.relnum }},
	{name: "scrolllock", what: "keep the cursor on the middle row (zl)",
		flag: func(m *Model) *bool { return &m.lock }},
	{name: "depth", what: "how far the document is open at the start (z1–z9 for now)",
		num: func(m *Model) *int { return &m.depth }, least: 1, def: 1,
		after: func(m *Model) {
			n := m.Cursor()
			m.tree.ExpandToDepth(m.depth)
			m.follow(n)
		}},
}

// lookup finds an option by its long name, or by its alias when typed.
func lookup(name string, alias bool) *option {
	for i := range options {
		if o := &options[i]; o.name == name || alias && o.short != "" && o.short == name {
			return o
		}
	}
	return nil
}

// show is how :set prints an option: `number`, `nonumber`, `depth=2`.
func (o *option) show(m *Model) string {
	if o.flag != nil {
		if *o.flag(m) {
			return o.name
		}
		return "no" + o.name
	}
	return o.name + "=" + strconv.Itoa(*o.num(m))
}

func (o *option) isDefault(m *Model) bool {
	if o.flag != nil {
		return !*o.flag(m)
	}
	return *o.num(m) == o.def
}

// assign checks a value against the option and sets it. Values come from :set (bool, int) or
// from TOML (bool, int64, float64, string, ...).
func (o *option) assign(m *Model, v any) error {
	if o.flag != nil {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("%s is true or false, not %s", o.name, quote(v))
		}
		*o.flag(m) = b
	} else {
		var n int
		switch v := v.(type) {
		case int:
			n = v
		case int64:
			n = int(v)
		default:
			return fmt.Errorf("%s is a whole number, not %s", o.name, quote(v))
		}
		if n < o.least {
			return fmt.Errorf("%s is %d or more, not %d", o.name, o.least, n)
		}
		*o.num(m) = n
	}
	if o.after != nil {
		o.after(m)
	}
	return nil
}

func quote(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return fmt.Sprint(v)
}

// SetOption sets an option by its long name, as config.toml and command-line flags do.
func (m *Model) SetOption(name string, v any) error {
	o := lookup(name, false)
	if o == nil {
		return fmt.Errorf("unknown option %q", name)
	}
	return o.assign(m, v)
}

// Configure applies config.toml. Whatever cannot be used is skipped and reported on the status
// line, so a typo in the file never stops jqv and never quietly does nothing.
func (m *Model) Configure(c config.Config) {
	m.configPath = c.Path
	problems := c.Problems
	for _, s := range c.Set {
		err := m.SetOption(s.Name, s.Value)
		if o := lookup(s.Name, true); err != nil && o != nil && o.name != s.Name {
			err = fmt.Errorf("unknown option %q (write %s)", s.Name, o.name)
		}
		if err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		m.status = filepath.Base(c.Path) + ": " + strings.Join(problems, "; ")
	}
}

// set runs :set with vim's forms: name, noname, invname, name!, name?, name=n; several at
// once; bare :set shows what differs from the defaults and :set all shows everything.
func (m *Model) set(args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		var changed []string
		for i := range options {
			if o := &options[i]; !o.isDefault(m) {
				changed = append(changed, o.show(m))
			}
		}
		m.status = strings.Join(changed, "  ")
		if m.status == "" {
			m.status = "all options at their defaults"
		}
		return
	}
	var shown []string
	for _, a := range fields {
		s, err := m.setOne(a)
		if err != nil {
			m.status = err.Error()
			return
		}
		if s != "" {
			shown = append(shown, s)
		}
	}
	m.status = strings.Join(shown, "  ")
}

// setOne handles one argument of :set, returning anything to show.
func (m *Model) setOne(a string) (string, error) {
	if a == "all" {
		var all []string
		for i := range options {
			all = append(all, options[i].show(m))
		}
		return strings.Join(all, "  "), nil
	}
	if name, val, ok := strings.Cut(a, "="); ok {
		o := lookup(name, true)
		switch {
		case o == nil:
			return "", fmt.Errorf("unknown option: %s", name)
		case o.flag != nil:
			return "", fmt.Errorf("%s is on or off: %s, no%s", o.name, o.name, o.name)
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return "", fmt.Errorf("%s is a whole number, not %q", o.name, val)
		}
		return "", o.assign(m, n)
	}
	if o := lookup(strings.TrimSuffix(a, "?"), true); o != nil {
		if o.num != nil || strings.HasSuffix(a, "?") { // a number option on its own shows its value
			return o.show(m), nil
		}
		return "", o.assign(m, true)
	}
	toggle := strings.HasSuffix(a, "!") || strings.HasPrefix(a, "inv")
	name := strings.TrimPrefix(strings.TrimSuffix(a, "!"), "inv")
	if !toggle {
		name = strings.TrimPrefix(a, "no")
	}
	o := lookup(name, true)
	switch {
	case o == nil || name == a:
		return "", fmt.Errorf("unknown option: %s", a)
	case o.num != nil:
		return "", fmt.Errorf("%s is a number, not on or off", o.name)
	case toggle:
		return "", o.assign(m, !*o.flag(m))
	default:
		return "", o.assign(m, false)
	}
}
