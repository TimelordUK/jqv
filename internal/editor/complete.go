package editor

import (
	"strconv"
	"strings"
)

// Candidate is one completion (Q-036): Insert replaces the span the Completer gave, and the
// cursor lands Back runes before its end (0: at the end; a skeleton puts it inside). Show is
// how it is listed, Insert if empty, and Hint a line about it, shown under the line.
type Candidate struct {
	Insert, Show, Hint string
	Back               int
}

// Completer gives the candidates for the line with the cursor at cur, and the span from..to
// of the line they replace. The editor knows nothing of what they are: a host's completer
// does (assist's knows jq and the REPL's commands).
type Completer func(line string, cur int) (from, to int, cands []Candidate)

// cycle is a completion in progress: Tab puts each candidate in place in turn over what was
// typed, shift+tab goes back, esc puts back what was typed, any other key keeps the one shown.
type cycle struct {
	orig     Buffer // the line as typed
	from, to int    // the span of orig the candidates replace
	cands    []Candidate
	i        int
}

// complete is Tab (dir 1) and shift+tab (-1): the first Tab asks the completer; one
// candidate goes straight in, with its hint shown until the next key; more cycle.
func (m *Model) complete(dir int) {
	if m.cyc != nil {
		n := len(m.cyc.cands)
		m.place((m.cyc.i + dir + n) % n)
		return
	}
	if m.Completer == nil {
		return
	}
	from, to, cands := m.Completer(m.buf.Text(), m.buf.cur)
	if len(cands) == 0 {
		return
	}
	from, to = min(max(from, 0), len(m.buf.text)), min(max(to, from), len(m.buf.text))
	m.cyc = &cycle{orig: m.buf, from: from, to: to, cands: cands}
	if dir > 0 {
		m.place(0)
	} else {
		m.place(len(cands) - 1)
	}
	if len(cands) == 1 {
		m.note, m.cyc = cands[0].Hint, nil
	}
}

// place puts candidate i over the typed span.
func (m *Model) place(i int) {
	c := m.cyc
	c.i = i
	ins := []rune(c.cands[i].Insert)
	t := append(append(append([]rune{}, c.orig.text[:c.from]...), ins...), c.orig.text[c.to:]...)
	m.buf.text = t
	m.buf.cur = c.from + max(len(ins)-c.cands[i].Back, 0)
}

// revertCycle is esc while cycling: the line as it was typed.
func (m *Model) revertCycle() {
	m.buf, m.cyc = m.cyc.orig, nil
}

// cycleView is what shows under the line while cycling: the candidates, the one in place
// reversed, as many as fit the width around it, then its hint.
func (m *Model) cycleView() string {
	c := m.cyc
	show := func(k int) string {
		if s := c.cands[k].Show; s != "" {
			return s
		}
		return c.cands[k].Insert
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	// pages of candidates that fit the width, two spaces apart; show the page with i on it
	start, end := 0, 0
	for {
		w, k := 0, start
		for ; k < len(c.cands); k++ {
			n := len([]rune(show(k))) + 2
			if w+n > width-12 && k > start { // room for "  … 12 more"
				break
			}
			w += n
		}
		end = k
		if c.i < end || end >= len(c.cands) {
			break
		}
		start = end
	}
	var b strings.Builder
	if start > 0 {
		b.WriteString("… ")
	}
	for k := start; k < end; k++ {
		if k > start {
			b.WriteString("  ")
		}
		if k == c.i {
			b.WriteString(reverse + show(k) + unreverse)
		} else {
			b.WriteString(show(k))
		}
	}
	if more := len(c.cands) - end; more > 0 {
		b.WriteString("  … " + strconv.Itoa(more) + " more")
	}
	s := "\n" + m.dim(b.String())
	if h := c.cands[c.i].Hint; h != "" {
		s += "\n" + m.dim(h)
	}
	return s
}

func (m *Model) dim(s string) string {
	if m.HintStyle == nil {
		return s
	}
	return m.HintStyle(s)
}
