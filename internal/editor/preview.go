package editor

import (
	"fmt"
	"strings"
)

// The preview (Q-054, DESIGN §24): a Previews action's result, shown under the entry and
// redrawn in place, so stepping along a line shows one current result instead of printing
// each above. It goes when the text changes (it would be stale) and when the entry is done;
// moving the cursor keeps it.

// previewMax is the most rows a preview takes when the terminal's height isn't known.
const previewMax = 10

// previewLines are the rows the preview shows: its first lines, each cut to one row, and how
// many more when it is longer than the room it has, a third of the terminal at most.
func (m *Model) previewLines() []string {
	if m.preview == "" || m.done {
		return nil
	}
	room := previewMax
	if m.height > 0 {
		room = min(previewMax, max(2, (m.height-spare)/3))
	}
	ls := strings.Split(strings.TrimRight(m.preview, "\n"), "\n")
	if len(ls) > room {
		ls = append(ls[:room-1:room-1], fmt.Sprintf("… %d more lines", len(ls)-(room-1)))
	}
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = m.oneRow(l)
	}
	return out
}
