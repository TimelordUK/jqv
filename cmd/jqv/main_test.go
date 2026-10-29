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
