package doc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// WriteJSON writes the subtree as JSON in document order, indented by indent per level as jq
// does (compact when indent is ""), with a newline after it. It is how a translated document
// (YAML, later XML) leaves jqv as JSON (C-015), to be piped into jq.
func (n *Node) WriteJSON(w io.Writer, indent string) error {
	bw := bufio.NewWriter(w)
	n.write(bw, indent, 0)
	bw.WriteByte('\n')
	return bw.Flush()
}

func (n *Node) write(w *bufio.Writer, indent string, depth int) {
	switch n.Kind {
	case String:
		writeString(w, n.Raw)
		return
	case Array, Object:
	default:
		w.WriteString(n.Raw)
		return
	}
	open, close := byte('['), byte(']')
	if n.Kind == Object {
		open, close = '{', '}'
	}
	w.WriteByte(open)
	if len(n.Children) == 0 {
		w.WriteByte(close)
		return
	}
	for i, c := range n.Children {
		if i > 0 {
			w.WriteByte(',')
		}
		newline(w, indent, depth+1)
		if n.Kind == Object {
			writeString(w, c.Key)
			w.WriteByte(':')
			if indent != "" {
				w.WriteByte(' ')
			}
		}
		c.write(w, indent, depth+1)
	}
	newline(w, indent, depth)
	w.WriteByte(close)
}

func newline(w *bufio.Writer, indent string, depth int) {
	if indent != "" {
		w.WriteByte('\n')
		w.WriteString(strings.Repeat(indent, depth))
	}
}

func writeString(w *bufio.Writer, s string) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.Encode(s)
	w.Write(bytes.TrimRight(b.Bytes(), "\n"))
}
