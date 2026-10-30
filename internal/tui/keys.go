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
		{"ctrl+f pgdown", "page down"},
		{"ctrl+b pgup", "page up"},
		{"ctrl+d", "half page down"},
		{"ctrl+u", "half page up"},
		{"gg home", "top"},
		{"G end", "bottom"},
		{"zz", "scroll cursor to middle"},
		{"zt zb", "scroll cursor to top / bottom"},
		{"ctrl+g", "where am I: row, % through the document, depth"},
	}},
	{"Blocks", []binding{
		{"[{ ]}", "start / end of the enclosing object"},
		{"%", "start of this block to its end, and back"},
		{"{ }", "previous / next block at this level, opened"},
		{"( )", "previous / next element of the array the cursor is in, opened"},
	}},
	{"Search", []binding{
		{"/", "search forward, keys and values"},
		{"?", "search back"},
		{"n N", "next / previous match"},
		{"z/", "fold everything except the matches"},
		{"esc", "clear highlight"},
		{"", `lower-case ignores case; a capital, or \C, matches case; \c ignores it`},
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
	{"Commands", []binding{
		{":", "command line"},
		{"", ":123 go to row · :set nu / rnu / nonu row numbers · :q quit"},
	}},
	{"", []binding{
		{"f1", "this help"},
		{"q ctrl+c", "quit"},
	}},
}
