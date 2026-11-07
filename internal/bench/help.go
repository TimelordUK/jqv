package bench

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/catalog"
	"github.com/TimelordUK/jqv/internal/query"
)

// manual answers `:help name` from jq's manual (Q-021): the entries for the name, their text,
// and their examples numbered for :try. A name nothing has is searched for in titles and text.
func (s *Session) manual(q string) string {
	c, err := catalog.Load()
	if err != nil {
		return "the manual didn't load: " + err.Error() + "\n"
	}
	switch q {
	case "jq":
		return s.contents(c)
	case "functions":
		fs := c.Functions()
		return s.wrap(fmt.Sprintf("jq's %d functions: ", len(fs)), fs) +
			":help a name for what it does; :help jq for operators and the rest, by section\n"
	}
	es, exact := c.Lookup(q)
	if len(es) == 0 {
		return fmt.Sprintf("nothing in jq's manual says %q (:help jq lists what it covers)\n", q)
	}
	var b strings.Builder
	if !exact {
		fmt.Fprintf(&b, "no function or topic called %q; the manual mentions it under:\n", q)
		for i, e := range es {
			if i == 15 {
				fmt.Fprintf(&b, "  … and %d more\n", len(es)-i)
				break
			}
			fmt.Fprintf(&b, "  %-44s :help %s\n", catalog.Plain(e.Title), helpName(e))
		}
		return b.String()
	}
	s.examples = nil
	for i, e := range es {
		if i > 0 {
			b.WriteByte('\n')
		}
		s.entry(&b, e)
	}
	if n := len(s.examples); n > 0 {
		first := "1"
		if n > 1 {
			first = "N"
		}
		fmt.Fprintf(&b, ":try %s runs an example on its own input; :try %s . on yours\n", first, first)
	}
	return b.String()
}

// entry writes one entry: title, text, what gojq lacks, examples numbered on from s.examples.
func (s *Session) entry(b *strings.Builder, e *catalog.Entry) {
	title := catalog.Plain(e.Title)
	if e.Section != e.Title {
		title += "  · " + e.Section
	}
	b.WriteString(title + "\n")
	if body := strings.TrimSpace(e.Body); body != "" {
		for _, l := range strings.Split(body, "\n") {
			b.WriteString(strings.TrimRight("  "+l, " ") + "\n")
		}
	}
	for _, n := range e.Names {
		if why, ok := catalog.NotInGojq[n]; ok {
			fmt.Fprintf(b, "  ! %s isn't in jqv: %s\n", n, why)
		}
	}
	if len(e.Examples) > 0 {
		b.WriteString("\n")
	}
	for _, x := range e.Examples {
		s.examples = append(s.examples, x)
		fmt.Fprintf(b, "  [%d] %s\n      on %s  →  %s\n", len(s.examples), x.Program, x.Input, strings.Join(x.Output, ", "))
		if x.Differs != "" {
			fmt.Fprintf(b, "      ! gojq differs: %s\n", x.Differs)
		}
	}
}

// helpName is what to type to see an entry: its first name, or its title.
func helpName(e *catalog.Entry) string {
	if len(e.Names) > 0 {
		return e.Names[0]
	}
	return strings.ToLower(catalog.Plain(e.Title))
}

// contents is :help jq: each section with the names under it.
func (s *Session) contents(c *catalog.Catalog) string {
	var b strings.Builder
	b.WriteString("jq's manual, by section. :help a name (:help trim, :help |) or a section (:help math)\n")
	for _, sec := range c.Sections() {
		var ns []string
		for _, e := range c.Section(sec) {
			if e.Title == sec {
				continue
			}
			if len(e.Names) == 0 {
				ns = append(ns, strings.ToLower(catalog.Plain(e.Title)))
			}
			ns = append(ns, e.Names...)
		}
		b.WriteString(s.wrap(sec+": ", ns))
	}
	b.WriteString("From the " + catalog.Source + ".\n")
	return b.String()
}

// wrap writes head then the words, comma-separated, onto lines of Width, indented under head.
func (s *Session) wrap(head string, words []string) string {
	if len(words) == 0 {
		return head + "text only, :help " + strings.ToLower(strings.TrimSuffix(head, ": ")) + "\n"
	}
	var b strings.Builder
	line := head
	for i, w := range words {
		if i < len(words)-1 {
			w += ","
		}
		if len(line)+1+len(w) > s.Width && line != "  " {
			b.WriteString(strings.TrimRight(line, " ") + "\n")
			line = "  "
		}
		if line != head && line != "  " {
			line += " "
		}
		line += w
	}
	return b.String() + line + "\n"
}

// try runs the arg'th example shown by the last :help: on its own input, and says whether the
// result is the manual's; or, with a trailing `.`, as a line on the current `.`. Either way it
// is then the last line, so :push makes it a stage.
func (s *Session) try(arg string) string {
	if len(s.examples) == 0 {
		return "no examples yet: :help a function first (:help trim)\n"
	}
	num, here := strings.CutSuffix(arg, ".")
	num = strings.TrimSpace(num)
	if num == "" && len(s.examples) == 1 {
		num = "1"
	}
	i, err := strconv.Atoi(num)
	if err != nil || i < 1 || i > len(s.examples) {
		return fmt.Sprintf("usage: :try N or :try N . with N from 1 to %d\n", len(s.examples))
	}
	x := s.examples[i-1]
	if here {
		return x.Program + "  on .\n" + s.eval(x.Program)
	}
	in, _, err := x.Values()
	if err != nil {
		return "the example's input isn't JSON: " + err.Error() + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  on %s\n", x.Program, x.Input)
	r := query.Run(x.Program, in, nil, s.Opt)
	for _, v := range r.Values {
		b.WriteString(s.format(v) + "\n")
	}
	if r.Err != nil {
		b.WriteString(failure(x.Program, r.Err))
	}
	if r.Err == nil && x.Same(r.Values) {
		b.WriteString("(as the manual says)\n")
	} else {
		b.WriteString("(the manual says " + strings.Join(x.Output, ", ") + ")\n")
		if x.Differs != "" {
			b.WriteString("! gojq differs: " + x.Differs + "\n")
		}
	}
	if r.Err == nil {
		s.remember(x.Program, r)
	}
	return b.String()
}
