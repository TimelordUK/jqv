// Package editor is the REPL's line editor (Q-025, DESIGN §18): text, a cursor, history and
// a keymap as data, as a Bubble Tea model. It knows keys and text and nothing about jq. What
// is jq-ish (help on a word, colour, completion) plugs in from outside as Handlers, so the core
// is tested alone: key messages in, text and cursor out, no terminal.
package editor

import "unicode"

// Buffer is the line being edited: its runes and the cursor, an index into them from 0 (before
// the first) to len (after the last).
type Buffer struct {
	text []rune
	cur  int
	kill []rune // the last text cut by a kill, for Yank
}

// Text is the line as a string.
func (b *Buffer) Text() string { return string(b.text) }

// Cursor is where the cursor is, in runes.
func (b *Buffer) Cursor() int { return b.cur }

// Set replaces the line and puts the cursor at its end.
func (b *Buffer) Set(s string) {
	b.text = []rune(s)
	b.cur = len(b.text)
}

// Insert puts rs before the cursor and moves the cursor past them.
func (b *Buffer) Insert(rs ...rune) {
	t := make([]rune, 0, len(b.text)+len(rs))
	t = append(append(append(t, b.text[:b.cur]...), rs...), b.text[b.cur:]...)
	b.text = t
	b.cur += len(rs)
}

// Backspace deletes the rune before the cursor.
func (b *Buffer) Backspace() {
	if b.cur > 0 {
		b.cut(b.cur-1, b.cur, false)
	}
}

// Delete deletes the rune under the cursor.
func (b *Buffer) Delete() {
	if b.cur < len(b.text) {
		b.cut(b.cur, b.cur+1, false)
	}
}

func (b *Buffer) Left() {
	if b.cur > 0 {
		b.cur--
	}
}

func (b *Buffer) Right() {
	if b.cur < len(b.text) {
		b.cur++
	}
}

func (b *Buffer) Home() { b.cur = 0 }
func (b *Buffer) End()  { b.cur = len(b.text) }

// WordLeft moves to the start of the word before the cursor, as ctrl+left does in a shell.
func (b *Buffer) WordLeft() { b.cur = b.wordStart() }

// WordRight moves to the end of the word after the cursor.
func (b *Buffer) WordRight() { b.cur = b.wordEnd() }

// KillToEnd cuts from the cursor to the end of the line (ctrl+k).
func (b *Buffer) KillToEnd() { b.cut(b.cur, len(b.text), true) }

// KillToStart cuts from the start of the line to the cursor (ctrl+u).
func (b *Buffer) KillToStart() { b.cut(0, b.cur, true) }

// KillWordBack cuts the word before the cursor (ctrl+w).
func (b *Buffer) KillWordBack() { b.cut(b.wordStart(), b.cur, true) }

// KillWordForward cuts the word after the cursor (ctrl+delete).
func (b *Buffer) KillWordForward() { b.cut(b.cur, b.wordEnd(), true) }

// Yank puts back the last text a kill cut (ctrl+y).
func (b *Buffer) Yank() { b.Insert(b.kill...) }

// cut removes text[i:j] and leaves the cursor at i; a kill keeps what it cut for Yank.
func (b *Buffer) cut(i, j int, kill bool) {
	if i >= j {
		return
	}
	if kill {
		b.kill = append([]rune(nil), b.text[i:j]...)
	}
	b.text = append(b.text[:i:i], b.text[j:]...)
	b.cur = i
}

// A word is letters, digits and _: `.people[].name` has the words people and name, so ctrl+w
// after it cuts name and leaves the dot.
func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (b *Buffer) wordStart() int {
	i := b.cur
	for i > 0 && !isWord(b.text[i-1]) {
		i--
	}
	for i > 0 && isWord(b.text[i-1]) {
		i--
	}
	return i
}

func (b *Buffer) wordEnd() int {
	i := b.cur
	for i < len(b.text) && !isWord(b.text[i]) {
		i++
	}
	for i < len(b.text) && isWord(b.text[i]) {
		i++
	}
	return i
}

// WordAt is the word around cursor in line, a word being a run of runes isWord takes. A host
// says what a word is: jq's has `@` and `$` in it, for `@csv` and `$ENV`.
func WordAt(line string, cursor int, isWord func(rune) bool) string {
	t := []rune(line)
	cursor = min(max(cursor, 0), len(t))
	i, j := cursor, cursor
	for i > 0 && isWord(t[i-1]) {
		i--
	}
	for j < len(t) && isWord(t[j]) {
		j++
	}
	return string(t[i:j])
}
