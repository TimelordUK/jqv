package editor

import (
	"fmt"
	"strings"
)

// History is the lines entered, oldest first, walked with up and down. Walking away from the
// line being typed keeps it, so down past the newest entry gives it back.
//
// A walk that starts with text on the line walks only the entries containing it, newest
// first, as zsh-history-substring-search does (R-011, DESIGN §18): type `def`, press up,
// and each press brings the next older line with `def` in it. Changing the line and pressing
// up again starts a new walk on the changed text.
type History struct {
	lines []string
	pos   int    // index into matches while walking; len(matches) is the line being typed
	draft string // the line being typed when the walk began, and what is matched
	walk  []int  // indices into lines that match draft, oldest first; nil when not walking
	shown string // the entry last given, to tell whether the line changed since
}

// Add appends a line entered, unless it is blank or the same as the newest, and ends any walk.
func (h *History) Add(line string) {
	if line != "" && (len(h.lines) == 0 || h.lines[len(h.lines)-1] != line) {
		h.lines = append(h.lines, line)
	}
	h.reset()
}

// Lines are the entries, oldest first.
func (h *History) Lines() []string { return append([]string(nil), h.lines...) }

// Prev gives the entry before the one shown, among those containing what was typed when the
// walk began; cur is the line as it is now. A cur that differs from the entry last shown
// starts a new walk on it. ok is false when there is no older match.
func (h *History) Prev(cur string) (string, bool) {
	if h.walk == nil || cur != h.shown {
		h.start(cur)
	}
	if h.pos == 0 {
		return "", false
	}
	h.pos--
	h.shown = h.lines[h.walk[h.pos]]
	return h.shown, true
}

// Next gives the entry after the one shown, then the line that was being typed. ok is false
// when not walking.
func (h *History) Next() (string, bool) {
	if h.walk == nil || h.pos >= len(h.walk) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.walk) {
		h.shown = h.draft
		return h.draft, true
	}
	h.shown = h.lines[h.walk[h.pos]]
	return h.shown, true
}

// Status says where a walk is, for the note under the line: `history 1/4 matching "def"`,
// counting from the newest; "" when not walking or back on the line being typed.
func (h *History) Status() string {
	if h.walk == nil || h.pos >= len(h.walk) {
		if h.walk != nil && len(h.walk) == 0 && h.draft != "" {
			return fmt.Sprintf("no history matching %q", h.draft)
		}
		return ""
	}
	s := fmt.Sprintf("history %d/%d", len(h.walk)-h.pos, len(h.walk))
	if h.draft != "" {
		s += fmt.Sprintf(" matching %q", h.draft)
	}
	return s
}

// start begins a walk from cur: the entries containing it (all of them when it is blank),
// leaving out any that are the same as cur, since bringing those back changes nothing.
func (h *History) start(cur string) {
	h.draft, h.shown = cur, cur
	h.walk = []int{}
	q := strings.TrimSpace(cur)
	for i, l := range h.lines {
		if l != q && strings.Contains(l, q) {
			h.walk = append(h.walk, i)
		}
	}
	h.pos = len(h.walk)
}

// reset ends a walk, for a new line.
func (h *History) reset() { h.walk, h.pos, h.draft, h.shown = nil, 0, "", "" }
