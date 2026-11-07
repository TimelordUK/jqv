package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	var out bytes.Buffer
	in := strings.NewReader(".web.restart\n")
	if err := run([]string{"repl", "../../testdata/yaml/anchors.yml"}, in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "anchors.yml · yaml→json") || !strings.Contains(out.String(), `"never"`) {
		t.Fatalf("got\n%s", out.String())
	}
}
