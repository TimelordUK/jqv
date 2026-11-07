package bench

import (
	"fmt"
	"strings"

	"github.com/TimelordUK/jqv/internal/config"
)

// Configure applies config.toml's [repl] table (DESIGN §18). As in the viewer, a bad entry is
// skipped and reported, never fatal; the problems include the file's own. [set] is the
// viewer's and is left alone.
func (s *Session) Configure(c config.Config) (problems []string) {
	problems = append(problems, c.Problems...)
	for _, st := range c.Repl {
		switch st.Name {
		case "pipe":
			p, ok := st.Value.(string)
			if !ok || strings.TrimSpace(p) == "" {
				problems = append(problems, fmt.Sprintf("[repl] pipe: want a command, as pipe = \"bat -l json\"; got %v", st.Value))
				continue
			}
			s.Pipe = strings.TrimSpace(p)
		default:
			problems = append(problems, fmt.Sprintf("[repl] %s: unknown setting (there is pipe)", st.Name))
		}
	}
	return problems
}
