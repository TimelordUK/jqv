package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/jqv/internal/bench"
	"github.com/TimelordUK/jqv/internal/config"
)

func TestPrintFromStdin(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"--print", "-d", "2", "-"}, strings.NewReader(`{"a":{"b":[1]},"c":"x"}`), &out)
	if err != nil {
		t.Fatal(err)
	}
	want := "▾ {2}\n  ▾ a {1}\n    ▸ b [1]\n    c: \"x\"\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}

func TestPrintFile(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--print", "../../testdata/orders.json"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "▾ {") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestReplReadsLinesFromStdin(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	var out bytes.Buffer
	in := strings.NewReader(":push .[1:][]\n.team\n:program\n")
	if err := run([]string{"repl", "../../testdata/people.json"}, in, &out); err != nil {
		t.Fatal(err)
	}
	want := `jqv repl · people.json · a line of jq runs on . and is forgotten; :push keeps it; :help
. ❯ :push .[1:][]
stage 1: .[1:][]  → object, 2 keys
.[1:][] ❯ .team
"beta"
.[1:][] ❯ :program
.[1:][]
` + ".[1:][] ❯ \n" // the prompt waiting at the end of input, then a newline to leave the line
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}

func TestReplNeedsAFile(t *testing.T) {
	err := run([]string{"repl"}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "needs a file") {
		t.Fatalf("err = %v", err)
	}
}

func TestBadJSONNamesTheInput(t *testing.T) {
	err := run([]string{"--print", "-"}, strings.NewReader(`{"a":`), &bytes.Buffer{})
	if err == nil || !strings.HasPrefix(err.Error(), "stdin:") {
		t.Fatalf("err = %v", err)
	}
}

func TestSizeLimit(t *testing.T) {
	big := `{"a":"` + strings.Repeat("x", 1<<20) + `"}` // just over 1 MB
	err := run([]string{"--print", "--max-size", "1", "-"}, strings.NewReader(big), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "more than 1 MB") {
		t.Fatalf("stdin over the limit: err = %v", err)
	}

	path := filepath.Join(t.TempDir(), "big.json")
	os.WriteFile(path, []byte(big), 0o644)
	err = run([]string{"--print", "--max-size", "1", path}, nil, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "big.json is 1 MB") {
		t.Fatalf("file over the limit: err = %v", err)
	}

	if err := run([]string{"--print", "--max-size", "2", path}, nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("under a raised limit: %v", err)
	}
}

func TestYAMLLoadsAsJSON(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--print-json", "../../testdata/yaml/order.yml"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile("../../testdata/yaml/order.json")
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); out.String() != w {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), w)
	}
}

func TestReplOnYAML(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	var out bytes.Buffer
	in := strings.NewReader(".web.restart\n")
	if err := run([]string{"repl", "../../testdata/yaml/anchors.yml"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "anchors.yml · yaml→json") || !strings.Contains(out.String(), `"never"`) {
		t.Fatalf("got\n%s", out.String())
	}
}

func TestReplReadsConfig(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("[repl]\npipe = \"definitely-not-a-program-jqv\"\nbogus = 1\n"), 0o644)
	t.Setenv("JQV_CONFIG", cfg)
	var out bytes.Buffer
	in := strings.NewReader(".[0].team\n:!\n")
	if err := run([]string{"repl", "../../testdata/people.json"}, in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"config: [repl] bogus: unknown setting", ":! definitely-not-a-program-jqv:"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}
}

// TestEditorKeys checks the REPL puts assist's jq-aware keys on the editor.
func TestEditorKeys(t *testing.T) {
	r, _ := lineEditor(bench.New(nil), strings.NewReader(""), &bytes.Buffer{}, config.Config{})
	for _, k := range []string{"f1", "f2", "f5", "ctrl+up", "ctrl+down"} {
		if a := r.Keys[k]; r.Handlers[a] == nil && r.Submits[a] == nil && r.Previews[a] == nil {
			t.Errorf("%s does nothing", k)
		}
	}
}

func TestJQCommand(t *testing.T) {
	for _, c := range []struct {
		h          handed
		file       string
		yaml, pwsh bool
		flags      []string
		want       string
	}{
		{handed{program: "."}, "people.json", false, true, nil, "jq . people.json"},
		{handed{program: ".[] | select(.name == \"Ann\")"}, `testdata\people.json`, false, true, nil, `jq '.[] | select(.name == "Ann")' testdata\people.json`},
		{handed{program: `.a | "it's"`}, "my file.json", false, true, nil, `jq '.a | "it''s"' 'my file.json'`},
		{handed{program: `.a | "it's"`}, "x.json", false, false, nil, `jq '.a | "it'\''s"' x.json`},
		{handed{program: ".jobs | keys"}, "ci.yml", true, true, nil, "jqv --print-json ci.yml | jq '.jobs | keys'"},
		// a program's arguments, a file as it is on disk, -c for :ps (Q-057)
		{handed{program: "$by[$root]", args: map[string]string{"root": "E 5", "min": "3"}}, "o.json", false, true, nil,
			`jq --arg min 3 --arg root 'E 5' '$by[$root]' o.json`},
		{handed{program: "…", file: `ex\org-chain\query.jq`, args: map[string]string{"root": "E5"}}, "o.json", false, true, []string{"-c"},
			`jq -c --arg root E5 -f ex\org-chain\query.jq o.json`},
	} {
		if got := jqCommand(c.h, c.file, c.yaml, c.pwsh, c.flags...); got != c.want {
			t.Errorf("jqCommand(%+v, %q)\n got  %s\n want %s", c.h, c.file, got, c.want)
		}
	}
}

// :ps (Q-057): the line as a pwsh command giving objects, printed (and copied on a terminal);
// the REPL carries on. A loaded file goes by -f, with its arguments.
func TestPS(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	var out bytes.Buffer
	in := strings.NewReader(".[] | {id, name}\n:ps Out-GridView\n:load ../../examples/jq/org-chain/query.jq\n:arg root E0150\n:ps\n.after\n")
	if err := run([]string{"repl", "../../testdata/orgchart.json"}, in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		":ps Out-GridView\njq -c '.[] | {id, name}' ../../testdata/orgchart.json | ConvertFrom-Json | Out-GridView\n",
		":ps\njq -c --arg root E0150 -f ../../examples/jq/org-chain/query.jq ../../testdata/orgchart.json | ConvertFrom-Json\n",
		"❯ .after", // carried on
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}
}

// TestWriteQuit runs :wq through the REPL: the command is printed and written to --emit's
// file, and piped lines never touch the clipboard.
func TestWriteQuit(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	emit := filepath.Join(t.TempDir(), "cmd.txt")
	var out bytes.Buffer
	in := strings.NewReader(":push .[]\n.people[] | .name\n:wq\n.never\n")
	if err := run([]string{"repl", "--emit", emit, "../../testdata/people.json"}, in, &out); err != nil {
		t.Fatal(err)
	}
	want := "jq '.[] | .people[] | .name' ../../testdata/people.json\n"
	if !strings.HasSuffix(out.String(), ".[] ❯ :wq\n"+want) {
		t.Errorf("output ends:\n%s", out.String())
	}
	if b, err := os.ReadFile(emit); err != nil || string(b) != want {
		t.Errorf("--emit file: %q %v", b, err)
	}
}

// With :raw on (Q-058), :wq hands back jq -r, so the shell prints what the REPL showed.
func TestWriteQuitRaw(t *testing.T) {
	t.Setenv("JQV_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	var out bytes.Buffer
	in := strings.NewReader(":raw\n.[0].people[0].name\n:wq\n")
	if err := run([]string{"repl", "../../testdata/people.json"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if want := ". [raw] ❯ :wq\njq -r '.[0].people[0].name' ../../testdata/people.json\n"; !strings.HasSuffix(out.String(), want) {
		t.Errorf("output ends:\n%s", out.String())
	}
}

// jqv config init writes the template for a new machine; again needs --force, which backs up
// the old one (owner 2026-11-19). jqv config alone says where the config is read from.
func TestConfigInit(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "jqv", "config.toml")
	t.Setenv("JQV_CONFIG", cfg)
	var out bytes.Buffer
	if err := run([]string{"config", "init"}, nil, &out); err != nil || !strings.Contains(out.String(), "wrote "+cfg) {
		t.Fatalf("init: %v, %q", err, out.String())
	}
	if err := run([]string{"config", "init"}, nil, &out); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("again without --force: %v", err)
	}
	out.Reset()
	if err := run([]string{"config", "init", "--force"}, nil, &out); err != nil || !strings.Contains(out.String(), "backed up to "+cfg+".bak") {
		t.Fatalf("--force: %v, %q", err, out.String())
	}
	out.Reset()
	if err := run([]string{"config"}, nil, &out); err != nil || !strings.Contains(out.String(), cfg) {
		t.Fatalf("config: %v, %q", err, out.String())
	}
}

// :keys in the REPL (owner 2026-11-19): the keys and what they do, from the config too; :keys
// show turns on printing each key as it comes (--show-keys inside the REPL), :keys show again
// off. Without jqv's editor (piped, --plain) there are no keys to show.
func TestKeysCommand(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("[keys]\nf6 = \":pp\"\n"), 0o644)
	t.Setenv("JQV_CONFIG", cfg)
	r, problems := lineEditor(bench.New(nil), strings.NewReader(""), &bytes.Buffer{}, config.Load(cfg))
	if len(problems) != 0 {
		t.Fatalf("problems %q", problems)
	}
	if got, ok := keysCommand(r, ":keys"); !ok || !strings.Contains(got, "f6         :pp") || !strings.Contains(got, cfg) {
		t.Errorf(":keys: %v %q", ok, got)
	}
	if got, _ := keysCommand(r, ":keys show"); !r.ShowKeys || !strings.Contains(got, "on") {
		t.Errorf(":keys show: %v %q", r.ShowKeys, got)
	}
	if got, _ := keysCommand(r, ":keys show"); r.ShowKeys || !strings.Contains(got, "off") {
		t.Errorf(":keys show again: %v %q", r.ShowKeys, got)
	}
	if _, ok := keysCommand(r, ":keysx"); ok {
		t.Error(":keysx isn't :keys")
	}
	if got, ok := keysCommand(newPlainLines(strings.NewReader(""), io.Discard, false), ":keys"); !ok || !strings.Contains(got, "jqv's line editor") {
		t.Errorf("without the editor: %q", got)
	}
}
