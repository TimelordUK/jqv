package editor

// Action is what a key does. The editor's own are below; a host adds its own (Help) with a
// Handler, and binds keys to them in the same table.
type Action string

const (
	Submit          Action = "submit"
	Interrupt       Action = "interrupt"     // abandon the line (ctrl+c)
	DeleteOrEOF     Action = "delete-or-eof" // ctrl+d: delete under the cursor, or end on an empty line
	Clear           Action = "clear-screen"  // ctrl+l, as bash, zsh and PSReadLine
	Revert          Action = "revert-line"   // esc: empty the line
	Backspace       Action = "backspace"
	Delete          Action = "delete"
	Left            Action = "left"
	Right           Action = "right"
	Home            Action = "home"
	End             Action = "end"
	WordLeft        Action = "word-left"
	WordRight       Action = "word-right"
	KillToEnd       Action = "kill-to-end"
	KillToStart     Action = "kill-to-start"
	KillWordBack    Action = "kill-word-back"
	KillWordForward Action = "kill-word-forward"
	Yank            Action = "yank"
	HistoryPrev     Action = "history-prev"
	HistoryNext     Action = "history-next"
	Complete        Action = "complete"      // tab: the next candidate (Q-036)
	CompletePrev    Action = "complete-prev" // shift+tab: the one before
)

// Keymap binds keys, as Bubble Tea names them ("ctrl+a", "up", "f1"), to actions (T-005).
type Keymap map[string]Action

// DefaultKeys are the shell's keys where there is one, and none that Zellij, tmux or Windows
// Terminal take first (DESIGN §5): no ctrl+b (tmux), ctrl+p/ctrl+n (Zellij modes), alt+f
// (Zellij floating panes) or alt+arrows (Zellij focus). Word moves are ctrl+arrows, as in
// PSReadLine.
func DefaultKeys() Keymap {
	return Keymap{
		"enter":          Submit,
		"ctrl+c":         Interrupt,
		"ctrl+d":         DeleteOrEOF,
		"ctrl+l":         Clear,
		"esc":            Revert,
		"backspace":      Backspace,
		"ctrl+h":         Backspace, // what some terminals send for backspace
		"delete":         Delete,
		"left":           Left,
		"right":          Right,
		"home":           Home,
		"ctrl+a":         Home,
		"end":            End,
		"ctrl+e":         End,
		"ctrl+left":      WordLeft,
		"ctrl+right":     WordRight,
		"ctrl+k":         KillToEnd,
		"ctrl+u":         KillToStart,
		"ctrl+w":         KillWordBack,
		"alt+backspace":  KillWordBack,
		"ctrl+backspace": KillWordBack,
		"ctrl+delete":    KillWordForward,
		"ctrl+y":         Yank,
		"up":             HistoryPrev,
		"down":           HistoryNext,
		"tab":            Complete,
		"shift+tab":      CompletePrev,
	}
}
