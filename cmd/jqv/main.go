// jqv: a terminal workbench for JSON, built around jq.
//
//	jqv file.json            open the viewer
//	cat file.json | jqv      read stdin; keys still come from the terminal
//	jqv --print -d 2 f.json  print the tree, opened to depth 2, and exit
//	jqv --print-json f.yml   write the document as JSON, in its key order, and exit
//	jqv repl file.json       the scratchpad REPL: lines of jq on the document (DESIGN §18)
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/config"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/tree"
	"github.com/TimelordUK/jqv/internal/tui"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev" // set by the release build: -ldflags "-X main.version=..."

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "jqv:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "repl" { // a file named repl opens as ./repl
		return runRepl(args[1:], stdin, stdout)
	}
	fs := flag.NewFlagSet("jqv", flag.ContinueOnError)
	print := fs.Bool("print", false, "print the tree as text and exit (no TUI)")
	printJSON := fs.Bool("print-json", false, "write the document as JSON (a .yml translated) and exit")
	depth := fs.Int("d", 1, "open the tree to this depth")
	showVersion := fs.Bool("version", false, "print the version and exit")
	maxMB := fs.Int64("max-size", defaultMaxMB, "largest document to load, in MB (it takes about 7x that in memory)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: jqv [--print | --print-json] [-d depth] [file.json | file.yml | -]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, "jqv", version)
		return nil
	}

	title, root, piped, err := load(fs.Arg(0), stdin, *maxMB)
	if err != nil {
		return err
	}

	if *printJSON { // a translated document out as JSON, for jq and friends (C-015)
		return root.WriteJSON(stdout, "  ")
	}
	m := tui.New(title, root)
	if *print { // the config is not read, so scripts and diffs don't depend on whose machine it is
		m.Tree().ExpandToDepth(*depth)
		return printTree(stdout, m.Tree())
	}

	m.Configure(config.Load(config.Path()))
	fs.Visit(func(f *flag.Flag) { // a flag given on the command line beats the config
		if f.Name == "d" {
			if err = m.SetOption("depth", *depth); err != nil {
				err = fmt.Errorf("-d: %w", err)
			}
		}
	})
	if err != nil {
		return err
	}
	m.Copy = clipboard.WriteAll
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if piped {
		opts = append(opts, tea.WithInputTTY()) // stdin is the document; keys come from the terminal
	}
	_, err = tea.NewProgram(m, opts...).Run()
	return err
}

// load reads the named document (or stdin), within the size limit. A .yml or .yaml file is
// translated to the same tree (C-013), and the title says so: "ci.yml · yaml→json".
func load(name string, stdin io.Reader, maxMB int64) (title string, root *doc.Node, piped bool, err error) {
	title, in, piped, err := input(name, stdin)
	if err != nil {
		return "", nil, false, err
	}
	defer in.Close()
	limit := maxMB << 20
	if f, ok := in.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode().IsRegular() && st.Size() > limit {
			return "", nil, false, tooBig(title, st.Size(), maxMB)
		}
	}
	lr := &io.LimitedReader{R: in, N: limit + 1} // stdin has no size up front: stop reading past the limit
	parse := doc.Parse
	if isYAML(name) {
		parse, title = doc.ParseYAML, title+" · yaml→json"
	}
	root, err = parse(bufio.NewReader(lr))
	if lr.N <= 0 {
		return "", nil, false, tooBig(title, -1, maxMB)
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("%s: %w", title, err)
	}
	return title, root, piped, nil
}

func isYAML(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yml", ".yaml":
		return true
	}
	return false
}

// runRepl is the scratchpad REPL (DESIGN §18): lines of jq on the document, read from stdin.
func runRepl(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("jqv repl", flag.ContinueOnError)
	maxMB := fs.Int64("max-size", defaultMaxMB, "largest document to load, in MB")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: jqv repl file.json   (lines of jq on the document; :help inside)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.Arg(0) == "" || fs.Arg(0) == "-" {
		return fmt.Errorf("jqv repl needs a file: stdin is where the lines of jq come from")
	}
	title, root, _, err := load(fs.Arg(0), stdin, *maxMB)
	if err != nil {
		return err
	}
	enableVT(stdout)
	return repl(title, bench.New(root.Value()), stdin, stdout, isTerminal(stdin))
}

// repl reads lines until :q or the end of input. When the lines are not typed (a script piped
// in), each is echoed after the prompt, so the output reads as a transcript.
func repl(title string, s *bench.Session, in io.Reader, out io.Writer, typed bool) error {
	w := bufio.NewWriter(out)
	defer w.Flush()
	fmt.Fprintf(w, "jqv repl · %s · a line of jq runs on . and is forgotten; :push keeps it; :help\n", title)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for {
		w.WriteString(s.Prompt())
		w.Flush()
		if !sc.Scan() {
			w.WriteString("\n")
			return errors.Join(sc.Err(), w.Flush())
		}
		line := sc.Text()
		if !typed {
			w.WriteString(line + "\n")
		}
		r := s.Do(line)
		if r.Clear {
			w.WriteString("\x1b[H\x1b[2J") // cursor home, clear the screen; scrollback stays
		}
		w.WriteString(r.Text)
		if r.Pipe != "" {
			w.Flush()
			if err := pipeTo(r.Pipe, r.Input, out); err != nil {
				fmt.Fprintf(w, ":! %s: %v\n", r.Pipe, err)
			}
		}
		if r.Quit {
			return w.Flush()
		}
	}
}

// pipeTo runs cmd (split on spaces, no shell) with input on its stdin and its output on ours.
// Only on a :! command, never per keystroke.
func pipeTo(cmd, input string, out io.Writer) error {
	f := strings.Fields(cmd)
	c := exec.Command(f[0], f[1:]...)
	c.Stdin = strings.NewReader(input)
	c.Stdout, c.Stderr = out, os.Stderr
	return c.Run()
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// defaultMaxMB keeps a document comfortably in memory: measured 2026-10-29, parsing costs about
// 11 ms and 7 MB of memory per MB of JSON, so 100 MB is about 1 s and 700 MB (DESIGN §11).
const defaultMaxMB = 100

func tooBig(title string, size int64, maxMB int64) error {
	what := "more than " + strconv.FormatInt(maxMB, 10) + " MB"
	if size >= 0 {
		what = strconv.FormatInt(size>>20, 10) + " MB"
	}
	return fmt.Errorf("%s is %s; jqv loads documents up to %d MB (about %d MB of memory). "+
		"Raise it with --max-size, or cut it down first (jq, head, grep for JSON lines)",
		title, what, maxMB, maxMB*7)
}

// input opens the named file, or stdin for "-" or no name when stdin is not a terminal.
func input(name string, stdin io.Reader) (title string, r io.ReadCloser, piped bool, err error) {
	if name == "" || name == "-" {
		if f, ok := stdin.(*os.File); ok && name == "" {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				return "", nil, false, fmt.Errorf("no input: give a file, or pipe JSON in (jqv -h for help)")
			}
		}
		return "stdin", io.NopCloser(stdin), true, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return "", nil, false, err
	}
	return filepath.Base(name), f, false, nil
}

func printTree(w io.Writer, t *tree.Tree) error {
	bw := bufio.NewWriter(w)
	for _, r := range t.Rows() {
		bw.WriteString(strings.Repeat("  ", r.Depth))
		bw.WriteString(t.Label(r))
		bw.WriteByte('\n')
	}
	return bw.Flush()
}
