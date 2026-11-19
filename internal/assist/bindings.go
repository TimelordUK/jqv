package assist

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/TimelordUK/jqv/internal/config"
	"github.com/TimelordUK/jqv/internal/editor"
	tea "github.com/charmbracelet/bubbletea"
)

// Binding is a key and what it does, as config.toml's [keys] writes it: an action ("help",
// "kill-to-end"), a command line (":!"), or "" for nothing.
type Binding struct{ Key, To string }

// Defaults are assist's keys (T-005, owner 2026-11-19), none taken by Zellij, tmux or Windows
// Terminal (DESIGN §5): F1 help on the word, F2 the last result into [repl] pipe (a bare :!,
// which needs the terminal, so it runs as a line and the line being typed comes back after),
// F3 the stack of what . is, F5 run to the cursor, F8 lay the entry out and join it back,
// ctrl+up/down along the pipeline, ctrl+] to the matching bracket. [keys] changes any of them.
var Defaults = []Binding{
	{"f1", string(Help)},
	{"f2", ":!"},
	{"f3", string(Scope)},
	{"f5", string(RunTo)},
	{"f8", string(Layout)},
	{"ctrl+up", string(StepBack)},
	{"ctrl+down", string(StepOn)},
	{"ctrl+]", string(Match)},
}

// BindKeys puts config.toml's [keys] on r, over the defaults. A mistake (no such key or
// action, a value that isn't text, enter or ctrl+c rebound) is a problem, said and skipped.
func BindKeys(r *editor.Reader, keys []config.Setting) (problems []string) {
	for _, k := range keys {
		to, ok := k.Value.(string)
		if !ok {
			problems = append(problems, fmt.Sprintf(`[keys] %s: want an action, a command or "", as %s; got %v`, k.Name, keyLine(Binding{k.Name, ":!"}), k.Value))
			continue
		}
		if err := bindKey(r, k.Name, to); err != nil {
			problems = append(problems, "[keys] "+k.Name+": "+err.Error())
		}
	}
	return problems
}

// bindKey binds key to to on r: a command line gets a Submitter of its own, which runs it as
// a line and keeps the line being typed; "" takes the key's binding away.
func bindKey(r *editor.Reader, key, to string) error {
	if !isKey(key) {
		return fmt.Errorf("no key %q: keys are named as Bubble Tea names them (f6, ctrl+o, alt+x, shift+up); --show-keys shows what a key sends", key)
	}
	if key == "enter" || key == "ctrl+c" {
		return fmt.Errorf("stays as it is: enter runs the line and ctrl+c abandons it")
	}
	a := editor.Action(to)
	switch {
	case to == "":
		delete(r.Keys, key)
		return nil
	case strings.HasPrefix(to, ":"):
		if r.Submits == nil {
			r.Submits = map[editor.Action]editor.Submitter{}
		}
		r.Submits[a] = func(string, int) string { return to }
	case !slices.Contains(actions(r), a):
		return fmt.Errorf("no action %q; there are %s, or a command such as \":pp\"", to, strings.Join(actionNames(actions(r)), ", "))
	}
	r.Keys[key] = a
	return nil
}

// actions are what a key can be bound to by name: the editor's own and the host's.
func actions(r *editor.Reader) []editor.Action {
	var as []editor.Action
	add := func(a editor.Action) {
		if !strings.HasPrefix(string(a), ":") && !slices.Contains(as, a) {
			as = append(as, a)
		}
	}
	for _, a := range editor.DefaultKeys() {
		add(a)
	}
	for a := range r.Handlers {
		add(a)
	}
	for a := range r.Submits {
		add(a)
	}
	for a := range r.Rewrites {
		add(a)
	}
	slices.Sort(as)
	return as
}

func actionNames(as []editor.Action) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = string(a)
	}
	return out
}

// keyNames are the keys Bubble Tea names, and the editor's own names for the ones it reads
// another way (ctrl+backspace).
var keyNames = func() map[string]bool {
	m := map[string]bool{"space": true}
	for t := tea.KeyType(-200); t < 300; t++ {
		if s := (tea.KeyMsg{Type: t}).String(); strings.TrimSpace(s) != "" {
			m[s] = true
		}
	}
	for k := range editor.DefaultKeys() {
		m[k] = true
	}
	return m
}()

// isKey: a key Bubble Tea names, alt+ one of those, or alt+ a character.
func isKey(k string) bool {
	if keyNames[k] {
		return true
	}
	if rest, ok := strings.CutPrefix(k, "alt+"); ok {
		return keyNames[rest] || len([]rune(rest)) == 1
	}
	return false
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// keyLine is a binding as a line of [keys]: a key with + or ] in it needs quotes in TOML.
func keyLine(b Binding) string {
	k := b.Key
	if !bareKey.MatchString(k) {
		k = fmt.Sprintf("%q", k)
	}
	return fmt.Sprintf("%s = %q", k, b.To)
}

// KeysText is :keys: every key and what it does now, jqv's own (assist's and config's) first,
// then the editor's, each by key.
func KeysText(r *editor.Reader) string {
	editors := map[editor.Action]bool{}
	for _, a := range editor.DefaultKeys() {
		editors[a] = true
	}
	ks := slices.Collect(maps.Keys(r.Keys))
	slices.SortFunc(ks, func(a, b string) int {
		ea, eb := editors[r.Keys[a]], editors[r.Keys[b]]
		if ea != eb {
			if eb {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	var b strings.Builder
	for _, k := range ks {
		fmt.Fprintf(&b, "%-10s %s\n", k, r.Keys[k])
	}
	return b.String()
}
