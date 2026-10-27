package tui

// binding is one line of the help overlay. Keep this table in step with Model.key and
// Model.chord; TestHelpCoversKeys checks that every key listed here does something.
type binding struct {
	keys string // space-separated, as KeyMsg.String() spells them; chords written "zR"
	what string
}

type keyGroup struct {
	title    string
	bindings []binding
}

var keymap = []keyGroup{
	{"Move", []binding{
		{"j down", "down"},
		{"k up", "up"},
		{"ctrl+d pgdown", "half page down"},
		{"ctrl+u pgup", "half page up"},
		{"gg home", "top"},
		{"G end", "bottom"},
	}},
	{"Open and close", []binding{
		{"l right", "open, or step into"},
		{"h left", "close, or go to parent"},
		{"enter space", "toggle"},
		{"L", "open everything under the cursor"},
		{"H", "fold everything under the cursor"},
		{"zo zc", "open / close subtree"},
		{"zR zM", "open / close whole document"},
		{"z1 z9", "open whole document to depth 1–9"},
	}},
	{"Copy", []binding{
		{"y", "jq path of the cursor node"},
		{"Y", "value of the cursor node as JSON"},
	}},
	{"", []binding{
		{"?", "this help"},
		{"q ctrl+c", "quit"},
	}},
}
