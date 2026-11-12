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
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/TimelordUK/jqv/internal/assist"
	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/config"
	"github.com/TimelordUK/jqv/internal/doc"
	"github.com/TimelordUK/jqv/internal/editor"
	"github.com/TimelordUK/jqv/internal/history"
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
	plain := fs.Bool("plain", false, "read lines with the console's own editing, not jqv's")
	showKeys := fs.Bool("show-keys", false, "print each key the line editor gets, to see what the terminal sends")
	emit := fs.String("emit", "", "on :wq, also write the jq command to this file (for a shell function to pick up)")
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
	s := bench.New(root.Value())
	s.Doc = root
	problems := s.Configure(config.Load(config.Path()))
	typed := isTerminal(stdin)
	var src lineSource = newPlainLines(stdin, stdout, !typed)
	h := handOver{file: fs.Arg(0), yaml: isYAML(fs.Arg(0)), emit: *emit}
	if typed {
		h.copy = clipboard.WriteAll
	}
	k := hooks{handBack: h.handBack}
	if typed && isTerminal(stdout) && !*plain {
		r := lineEditor(s, stdin, stdout)
		r.ShowKeys = *showKeys
		// history only for lines typed into the editor: scripts and tests never touch it
		path := history.Path()
		lines, err := history.Recall(path, fs.Arg(0), 1000)
		if err != nil {
			k.notes = append(k.notes, "history: "+err.Error())
		}
		for _, l := range lines {
			r.History.Add(l)
		}
		k.log = history.Open(path, fs.Arg(0))
		src = r
	}
	return repl(title, s, problems, src, stdout, k)
}

// hooks are what the REPL does beyond the session: hand :wq's program back, and keep the
// history (R-001; nil when lines aren't typed into the editor).
type hooks struct {
	handBack func(program string) string
	log      *history.Log
	notes    []string // said under the banner
}

// lineSource gives the REPL its lines: the plain reader, or the line editor (Q-025).
type lineSource interface {
	ReadLine(prompt string) (string, error) // io.EOF at the end
}

// plainLines reads whole lines, after enter, with the console's own editing. When the lines
// are not typed (a script piped in), each is echoed after the prompt, so the output reads as a
// transcript.
type plainLines struct {
	sc   *bufio.Scanner
	out  io.Writer
	echo bool
}

func newPlainLines(in io.Reader, out io.Writer, echo bool) *plainLines {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	return &plainLines{sc, out, echo}
}

func (p *plainLines) ReadLine(prompt string) (string, error) {
	io.WriteString(p.out, prompt)
	if !p.sc.Scan() {
		io.WriteString(p.out, "\n")
		return "", cmp.Or(p.sc.Err(), io.EOF)
	}
	line := p.sc.Text()
	if p.echo {
		io.WriteString(p.out, line+"\n")
	}
	return line, nil
}

// lineEditor is jqv's own line editor on the terminal. The editor knows no jq: the jq-aware
// keys (F1 help, F5 run to the cursor, ctrl+up/down along the pipeline) come from assist.
func lineEditor(s *bench.Session, in io.Reader, out io.Writer) *editor.Reader {
	r := &editor.Reader{In: in, Out: out, Keys: editor.DefaultKeys()}
	assist.Bind(r, s)
	return r
}

// repl runs lines until :q or :wq or the end of input.
func repl(title string, s *bench.Session, problems []string, src lineSource, out io.Writer, k hooks) error {
	w := bufio.NewWriter(out)
	defer w.Flush()
	fmt.Fprintf(w, "jqv repl · %s · a line of jq runs on . and is forgotten; :push keeps it; :help\n", title)
	for _, p := range problems { // a bad config is said, never fatal (DESIGN §13)
		fmt.Fprintf(w, "config: %s\n", p)
	}
	for _, n := range k.notes {
		fmt.Fprintln(w, n)
	}
	warned := false
	for {
		w.Flush()
		line, err := src.ReadLine(s.Prompt())
		switch {
		case errors.Is(err, editor.ErrInterrupted):
			continue
		case errors.Is(err, io.EOF):
			return w.Flush()
		case err != nil:
			return errors.Join(err, w.Flush())
		}
		start := time.Now()
		r := s.Do(line)
		if err := k.log.Add(strings.TrimSpace(line), r.Failed, time.Since(start)); err != nil && !warned {
			fmt.Fprintf(w, "history: %v (not saved; said once)\n", err)
			warned = true
		}
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
		if r.Emit != "" {
			w.WriteString(k.handBack(r.Emit))
		}
		if r.Quit {
			return w.Flush()
		}
	}
}

// handOver is :wq's program as a command for the shell: printed, written to --emit's file
// for a shell function to put in its history, and copied when the lines are typed.
type handOver struct {
	file string // the document, as named on the command line
	yaml bool
	emit string // --emit FILE, or ""
	copy func(string) error
}

func (h handOver) handBack(program string) string {
	c := jqCommand(program, h.file, h.yaml, runtime.GOOS == "windows")
	out := c + "\n"
	if h.emit != "" {
		if err := os.WriteFile(h.emit, []byte(c+"\n"), 0o644); err != nil {
			out += "--emit: " + err.Error() + "\n"
		}
	}
	if h.copy != nil && h.copy(c) == nil {
		out += "(copied)\n"
	}
	return out
}

// jqCommand is jq running program on file, quoted for pwsh or for sh. jq can't read YAML,
// so a YAML file goes through jqv --print-json first.
func jqCommand(program, file string, yaml, pwsh bool) string {
	q := func(s string) string {
		if s != "" && strings.IndexFunc(s, func(r rune) bool {
			return !(r == '.' || r == '_' || r == '-' || r == '/' || r == '\\' || r == ':' ||
				unicode.IsLetter(r) || unicode.IsDigit(r))
		}) < 0 {
			return s
		}
		if pwsh {
			return "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	if yaml {
		return "jqv --print-json " + q(file) + " | jq " + q(program)
	}
	return "jq " + q(program) + " " + q(file)
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

func isTerminal(r any) bool {
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
