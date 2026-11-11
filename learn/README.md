# Learning jq, checked

Worked examples, laid out like a notebook: the input, the query, and what it gives. Each
query's output is checked by `go test` against gojq (what jqv runs) and real jq, so a page can't
quietly drift from what jq does.

1. [Paths: getting to a value](01-paths.md)
2. [Strings: shaping a value before you test it](02-strings.md)
3. [Isolating records: `select`](03-select.md)
4. [Arrays: one, some, or each](04-arrays.md) (index, slice, iterate)
5. [Generators: many outputs, joined and nested](05-generators.md) (`,`, `range` up and down,
   `as $i`, every combination)
6. [Choosing: `if … then … else … end`](06-if.md) (`elif`, no `else`, what counts as true,
   `//`, `if` versus `select`)
7. [Groups and sets: `unique`, `group_by`, `-`, `IN`](07-groups.md) (repeats in a Path, with
   and without case, keeping the order, `contains` versus `IN`, `INDEX`)

These pages are also where jqv's reader is worked out (DESIGN §16): each one ends with what
the page means for jqv.

## Trying things

Add a query to a page, a fenced block with `jq` after the opening fence, and run

```
./scripts/learn.ps1
```

Its output is written in under it. Run it again after changing a query or an input and the
outputs follow. To test yourself, write the output you expect, then run
`./scripts/learn.ps1 -Check`: it fails, showing what jq really gives, wherever you were wrong.

`go test ./...` checks these pages along with everything else.

## How a page is read

- A `json` block is an input. The queries after it run on it, until the next input.
- A line `Input: [name](../testdata/file.json)` makes a file the input.
- A `jq` block is a query. `jq -r`, `jq -c` and `jq -n` work as they do on the command line.
- The `json output` block after a query is what it gives (`text output` under `-r`), one value
  after another. An empty one means nothing came out.
- A `text error` block means the query fails. The text is jq's message; only the failing is
  checked, since gojq words its errors differently.
- Everything else is prose.

The outputs are written by real jq when it's on the PATH, so objects keep jq's key order.
Without it they come from gojq, which sorts keys. The check ignores key order.

Lessons are named `NN-topic.md`. This README isn't run.
