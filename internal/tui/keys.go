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
		{"zl", "lock the cursor to the middle row, or unlock"},
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
		{"f", "finder: every match as a list, the tree following the selection"},
		{"F", "the last search as a list, on the match at the cursor"},
		{"z/", "fold everything except the matches"},
		{"esc", "clear highlight"},
		{"", `smartcase, \c \C force it · "AE" matches a whole key or string`},
		{"", "in the list: up down ctrl+j ctrl+k pgup pgdown move · enter land · esc back"},
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
	{"Pick (DESIGN §26)", []binding{
		{"space", "on a field in a record: pick it into a table, again to unpick"},
		{"x", "on an array of values in a record, or a value in one: a row for each, beside the picks (cross join); again to undo"},
		{"f6", "move between the tree and the table"},
		{"f7", "hide the table, or show it again"},
		{"f9", "zoom the pane with focus"},
		{"", "in the table: j k up down pgup pgdown G home move · h l left right 0 $ columns"},
	}},
	{"Stages (DESIGN §28)", []binding{
		{"J K", "the stage cursor: the tree shows what goes into a stage, the table what comes out; J past the last locks it"},
		{"=", "at the end, on a value: a filter to it as a new stage; on a filter, added with and"},
		{"|", "on a filter, a value going in: added to it with or"},
		{"!", "as = but anything except this value: != ; on a filter, added with and"},
		{"~", "on a filter: turn it round, everything it let through left out; again to undo"},
		{"D", "delete the stage under the cursor"},
		{"u", "undo the last change to the program"},
	}},
	{"Copy", []binding{
		{"y", "jq path of the cursor node"},
		{"Y", "value of the cursor node: a string as its text, anything else as JSON"},
	}},
	{"Commands", []binding{
		{":", "command line"},
		{"", ":123 go to row · :set an option (below) · :q quit"},
	}},
	{"", []binding{
		{"f1", "this help"},
		{"q ctrl+c", "quit"},
	}},
}
