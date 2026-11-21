package assist

import (
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/jqsyntax"
	"github.com/TimelordUK/jqv/internal/scope"
	"github.com/charmbracelet/lipgloss"
)

// Palette is how the line is coloured (Q-027, DESIGN §19): a style for each kind of token,
// and the brackets' styles, taken in turn by depth, so a pair has one colour and its
// neighbours others. A kind with no style is drawn plain. Error is also what a bracket that
// closes nothing gets.
type Palette struct {
	Kinds    map[jqsyntax.Kind]func(string) string
	Brackets []func(string) string
	Checks   map[scope.Level]func(string) string // a field or index a check found (Q-046), by level; none for a level leaves it as it was
	// Recursive is a def that calls itself, its name and its calls to itself, and the builtins
	// that recurse (recurse, walk, ..), so recursion shows at a glance (Q-052); nil leaves them
	Recursive func(string) string
}

// Highlight is the editor's Painter for jq: the line's jq (the whole line, or the argument of
// :push, :pp, :r, :tree, :lex, :fmt) read by jqsyntax.Lex and coloured by p. Other commands stay plain.
func Highlight(p Palette) editor.Painter {
	return func(line string) []editor.Span {
		off, expr, ok := jqPart(line)
		if !ok {
			return nil
		}
		return p.spans(expr, off)
	}
}

// Paint colours jq text as the line is coloured, for what the REPL prints (:fmt's layout).
func Paint(p Palette) func(string) string {
	return func(text string) string {
		return editor.Render([]rune(text), p.spans(text, 0), -1)
	}
}

// spans are expr's tokens with their styles, off runes along the line.
func (p Palette) spans(expr string, off int) []editor.Span {
	var spans []editor.Span
	rs := []rune(expr)
	rec := map[[2]int]bool{} // the tokens that are recursion
	if p.Recursive != nil {
		r := jqsyntax.Recursive(expr)
		for _, d := range r.Defs {
			rec[d.Name] = true
			for _, c := range d.Calls {
				rec[c] = true
			}
		}
		for _, b := range r.Builtins {
			rec[b] = true
		}
	}
	for _, t := range jqsyntax.Lex(expr) {
		style := p.Kinds[t.Kind]
		if rec[[2]int{t.Pos, t.End}] {
			style = p.Recursive
		}
		if t.Text == "`" && (t.End == len(rs) || rs[t.End] == '\n' || rs[t.End] == '\r') {
			style = p.Kinds[jqsyntax.Comment] // enter breaks the line there (Q-026): not an error
		}
		if (t.Kind == jqsyntax.Bracket || t.Kind == jqsyntax.Interp) && len(p.Brackets) > 0 {
			style = p.Brackets[t.Depth%len(p.Brackets)]
		}
		if style != nil {
			spans = append(spans, editor.Span{From: off + t.Pos, To: off + t.End, Style: style})
		}
	}
	return spans
}

// Colours is the default palette, the tree view's (internal/tui): fields and keys as its
// keys, strings and numbers as its values, so a line reads like the document it runs on.
func Colours() Palette {
	fg := func(light, dark string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: light, Dark: dark})
	}
	field := fg("#1E66F5", "#89B4FA")
	str := fg("#40A02B", "#A6E3A1")
	k := map[jqsyntax.Kind]lipgloss.Style{
		jqsyntax.Field:    field,
		jqsyntax.Key:      field,
		jqsyntax.Dot:      field,
		jqsyntax.Recurse:  field,
		jqsyntax.Quote:    str,
		jqsyntax.Text:     str,
		jqsyntax.Escape:   fg("#179299", "#94E2D5"),
		jqsyntax.Number:   fg("#FE640B", "#FAB387"),
		jqsyntax.Keyword:  fg("#8839EF", "#CBA6F7"),
		jqsyntax.Name:     fg("#04A5E5", "#89DCEB"),
		jqsyntax.Variable: fg("#E64553", "#EBA0AC"),
		jqsyntax.Format:   fg("#EA76CB", "#F5C2E7"),
		jqsyntax.Pipe:     fg("#7C7F93", "#9399B2").Bold(true),
		jqsyntax.Operator: fg("#7C7F93", "#9399B2"),
		jqsyntax.Punct:    fg("#7C7F93", "#9399B2"),
		jqsyntax.Comment:  fg("#9CA0B0", "#6C7086").Italic(true),
		jqsyntax.Error:    fg("#D20F39", "#F38BA8").Underline(true),
	}
	p := Palette{Kinds: map[jqsyntax.Kind]func(string) string{}}
	for kind, st := range k {
		p.Kinds[kind] = render(st)
	}
	p.Checks = map[scope.Level]func(string) string{
		scope.Error:   render(fg("#D20F39", "#F38BA8").Underline(true)),
		scope.Warning: render(fg("#DF8E1D", "#F9E2AF").Underline(true)),
	}
	// recursion: a colour no other token has (flamingo), bold and italic, not underlined as a check is
	p.Recursive = render(fg("#DD7878", "#F2CDCD").Bold(true).Italic(true))
	for _, c := range [][2]string{{"#DF8E1D", "#F9E2AF"}, {"#8839EF", "#CBA6F7"}, {"#04A5E5", "#89DCEB"}} {
		p.Brackets = append(p.Brackets, render(fg(c[0], c[1])))
	}
	return p
}

func render(st lipgloss.Style) func(string) string {
	return func(s string) string { return st.Render(s) }
}
