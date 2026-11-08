// Package learn holds the jq lessons (DESIGN §17, Q-019): markdown files (01-paths.md, ...;
// README.md is not a lesson) where each query sits
// between the input it runs on and the output it gives, so they read on GitHub like a notebook.
// This test runs every query through gojq and, when it is on the PATH, real jq, and fails when
// an output block says anything else. `go test ./learn -update` writes the outputs, from real jq
// when there is one.
//
// In a lesson:
//
//	```json                  an input: the queries below it run on this, until the next input
//	Input: [x](../a.json)    an input from a file, as a line of its own
//	```jq                    a query; ```jq -r, -c and -n work as they do on the command line
//	```json output           what the query above gives (```text output for -r)
//	```text error            the query fails; the text is the error, but only the failing is checked
package learn

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

var update = flag.Bool("update", false, "write the output blocks")

// block is one fenced block of a lesson: its info string, its text, and the lines it spans
// (the fences included).
type block struct {
	info        string
	text        string
	first, last int
}

var fileInput = regexp.MustCompile(`^Input: \[[^\]]*\]\(([^)]+)\)\s*$`)

// step is a query with the input it runs on and the output block after it, if any.
type step struct {
	query  block
	flags  []string
	input  []byte
	output *block
}

// parse finds the steps of a lesson.
func parse(path string, lines []string) ([]step, error) {
	var (
		steps []step
		input []byte
		open  *block
	)
	for i, l := range lines {
		if open != nil {
			if strings.TrimSpace(l) == "```" {
				open.last = i
				b := *open
				open = nil
				switch {
				case b.info == "json":
					input = []byte(b.text)
				case b.info == "jq" || strings.HasPrefix(b.info, "jq "):
					if input == nil && !strings.Contains(b.info, "-n") {
						return nil, fmt.Errorf("%s:%d: a query with no input above it", path, b.first+1)
					}
					steps = append(steps, step{query: b, flags: strings.Fields(b.info)[1:], input: input})
				case strings.HasSuffix(b.info, " output") || strings.HasSuffix(b.info, " error"):
					if len(steps) == 0 || steps[len(steps)-1].output != nil {
						return nil, fmt.Errorf("%s:%d: an output with no query above it", path, b.first+1)
					}
					steps[len(steps)-1].output = &b
				}
				continue
			}
			open.text += l + "\n"
			continue
		}
		if strings.HasPrefix(l, "```") {
			open = &block{info: strings.TrimSpace(strings.TrimPrefix(l, "```")), first: i}
			continue
		}
		if m := fileInput.FindStringSubmatch(l); m != nil {
			b, err := os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(m[1])))
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
			}
			input = b
		}
	}
	if open != nil {
		return nil, fmt.Errorf("%s:%d: a block that never closes", path, open.first+1)
	}
	return steps, nil
}

// has says whether a short flag such as -n is given, alone or combined as jq allows (-nc).
func has(flags []string, f string) bool {
	for _, g := range flags {
		if g == f {
			return true
		}
		if len(f) == 2 && len(g) > 2 && g[0] == '-' && g[1] != '-' && strings.Contains(g[1:], f[1:]) {
			return true
		}
	}
	return false
}

// runGojq gives the query's outputs, or the first error.
func runGojq(s step) ([]any, error) {
	q, err := gojq.Parse(s.query.text)
	if err != nil {
		return nil, err
	}
	var in any
	if !has(s.flags, "-n") {
		d := json.NewDecoder(bytes.NewReader(s.input))
		d.UseNumber()
		if err := d.Decode(&in); err != nil {
			return nil, fmt.Errorf("input: %w", err)
		}
	}
	var out []any
	it := q.Run(in)
	for v, ok := it.Next(); ok; v, ok = it.Next() {
		if err, ok := v.(error); ok {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}

// format writes values as jq would (less its key order, which gojq does not keep).
func format(vs []any, flags []string) string {
	var b strings.Builder
	for _, v := range vs {
		if s, ok := v.(string); ok && has(flags, "-r") {
			b.WriteString(s + "\n")
			continue
		}
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		if !has(flags, "-c") {
			e.SetIndent("", "  ")
		}
		e.Encode(v)
	}
	return b.String()
}

// runJQ gives real jq's output text, or its error.
func runJQ(jq string, s step) (string, error) {
	args := append(append([]string{}, s.flags...), s.query.text)
	cmd := exec.Command(jq, args...)
	cmd.Stdin = bytes.NewReader(s.input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), nil
}

// values reads what an output block says, as plain values to compare. Under -r a line is a
// string unless it is JSON.
func values(text string, flags []string) ([]any, error) {
	var out []any
	if has(flags, "-r") && text != "" {
		for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			var v any
			if json.Unmarshal([]byte(l), &v) != nil {
				v = l
			}
			out = append(out, v)
		}
		return out, nil
	}
	d := json.NewDecoder(strings.NewReader(text))
	for {
		var v any
		if err := d.Decode(&v); err == io.EOF {
			return out, nil
		} else if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func same(a, b string, flags []string) bool {
	va, err := values(a, flags)
	if err != nil {
		return false
	}
	vb, err := values(b, flags)
	return err == nil && reflect.DeepEqual(va, vb)
}

func TestLessons(t *testing.T) {
	files, _ := filepath.Glob("[0-9]*.md")
	if len(files) == 0 {
		t.Fatalf("no lessons beside the test")
	}
	jq, _ := exec.LookPath("jq")
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
			steps, err := parse(f, lines)
			if err != nil {
				t.Fatal(err)
			}
			var edits []step
			for _, s := range steps {
				where := fmt.Sprintf("%s:%d: %s", f, s.query.first+2, strings.TrimSpace(s.query.text))
				got, gerr := runGojq(s)
				want := format(got, s.flags)
				if jq != "" {
					text, jerr := runJQ(jq, s)
					if (gerr == nil) != (jerr == nil) {
						t.Errorf("%s\n gojq and jq disagree: gojq %v, jq %v", where, gerr, jerr)
						continue
					}
					if gerr == nil && !same(text, want, s.flags) {
						t.Errorf("%s\n gojq and jq disagree:\n gojq %s jq %s", where, want, text)
						continue
					}
					want, gerr = text, jerr
				}
				if gerr != nil {
					if s.output == nil || !strings.HasSuffix(s.output.info, " error") {
						if *update {
							if s.output == nil {
								s.output = &block{first: -1}
							}
							s.output.info, s.output.text = "text error", gerr.Error()+"\n"
							edits = append(edits, s)
							continue
						}
						t.Errorf("%s\n fails: %v", where, gerr)
					} else if *update && s.output.text != gerr.Error()+"\n" {
						s.output.text = gerr.Error() + "\n"
						edits = append(edits, s)
					}
					continue
				}
				kind := "json output"
				if has(s.flags, "-r") {
					kind = "text output"
				}
				if s.output != nil && s.output.info == kind && same(s.output.text, want, s.flags) {
					if *update && s.output.text != want {
						s.output.text = want
						edits = append(edits, s)
					}
					continue
				}
				if *update {
					if s.output == nil {
						s.output = &block{info: kind, first: -1}
					}
					s.output.info, s.output.text = kind, want
					edits = append(edits, s)
					continue
				}
				if s.output == nil {
					t.Errorf("%s\n has no output block (run with -update to write it):\n%s", where, want)
				} else {
					t.Errorf("%s\n says:\n%s gives:\n%s", where, s.output.text, want)
				}
			}
			if *update && len(edits) > 0 {
				if err := os.WriteFile(f, []byte(rewrite(lines, edits)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// rewrite puts each edited step's output block in place, after its query.
func rewrite(lines []string, edits []step) string {
	var out []string
	next := 0
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		if next >= len(edits) || i != edits[next].query.last {
			continue
		}
		o := edits[next].output
		next++
		fresh := []string{"```" + o.info}
		if o.text != "" {
			fresh = append(fresh, strings.Split(strings.TrimSuffix(o.text, "\n"), "\n")...)
		}
		fresh = append(fresh, "```")
		if o.first < 0 {
			out = append(append(out, ""), fresh...)
			continue
		}
		// Keep whatever sits between the query and its old output; replace the output.
		out = append(out, lines[i+1:o.first]...)
		out = append(out, fresh...)
		i = o.last
	}
	return strings.Join(out, "\n")
}
