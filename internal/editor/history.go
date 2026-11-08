package editor

// History is the lines entered, oldest first, walked with up and down. Walking away from the
// line being typed keeps it, so down past the newest entry gives it back.
type History struct {
	lines []string
	pos   int    // index into lines while walking; len(lines) is the line being typed
	draft string // the line being typed when the walk began
}

// Add appends a line entered, unless it is blank or the same as the newest, and ends any walk.
func (h *History) Add(line string) {
	if line != "" && (len(h.lines) == 0 || h.lines[len(h.lines)-1] != line) {
		h.lines = append(h.lines, line)
	}
	h.pos = len(h.lines)
}

// Lines are the entries, oldest first.
func (h *History) Lines() []string { return append([]string(nil), h.lines...) }

// Prev gives the entry before the one shown; cur is the line as it is now, kept as the draft
// when the walk starts. ok is false at the oldest entry.
func (h *History) Prev(cur string) (string, bool) {
	if h.pos == 0 {
		return "", false
	}
	if h.pos == len(h.lines) {
		h.draft = cur
	}
	h.pos--
	return h.lines[h.pos], true
}

// Next gives the entry after the one shown, then the draft. ok is false when not walking.
func (h *History) Next() (string, bool) {
	if h.pos >= len(h.lines) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.lines) {
		return h.draft, true
	}
	return h.lines[h.pos], true
}

// reset ends a walk, for a new line.
func (h *History) reset() { h.pos = len(h.lines); h.draft = "" }
