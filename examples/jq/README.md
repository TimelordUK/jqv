# jq samples

Small, real jq queries worth keeping: each folder holds the query, an input, and the output
real jq gives. `go test ./examples/...` runs every one through gojq (the engine jqv embeds)
and fails if the output changes, so they stay working and show any gojq/jq difference.
These are the seed for jqv's recipes (R-002, R-005).

| Sample | What it shows |
|---|---|
| [fix-dictionary](fix-dictionary/query.jq) | A FIX dictionary (QuickFIX XML converted to JSON) made into what an application uses: fields by name and tag, messages by MsgType, components and groups resolved recursively where they're used. `items` copes with the converter's lone-child objects. Input is a cut of `testdata/FIX4.4.json` (3 messages) |
| [fix-message](fix-message/query.jq) | One message's definition as an indented outline (tag, name, type, `*` required), read from fix-dictionary's output: two programs composed |
| [follow-order-tree](follow-order-tree/query.jq) | Every message in one order's tree, from a log where children name only their parent. One streaming pass, holding only the id set (DESIGN §11, R-007) |
| [org-below](org-below/query.jq) | Everyone below one person in a flat org chart, through solid and dotted lines: `recurse` over an `INDEX(.id)` lookup, `unique` for people reached twice |
| [org-chain](org-chain/query.jq) | The management chain up to the top, one array per route: a recursive def that is also a generator |
| [org-tree](org-tree/query.jq) | A flat org chart as a nested tree, solid lines nested and dotted lines listed by id. Try any of the three on `testdata/orgchart.json` |
| [order-rows](order-rows/query.jq) | Pick fields, flatten a nested object, filter: rows ready for a grid |
| [sum-filled-qty](sum-filled-qty/query.jq) | Sum a nested array per item and in total, with `// 0` for empty arrays |
| [team-report](team-report/query.jq) | A program laid out as a script, commented: `def` helpers (value and filter parameters), data sets named with `as` and glued into one result, `--arg` from the command line |

**Adding one:** make a folder with `query.jq` (a comment at the top: what it's for and the
command line), `input.json` or `input.jsonl`, and `case.json` if it needs `--arg`s or `-n`:

```json
{"args": {"root": "P1"}, "null_input": true}
```

Then produce the expected output **with real jq**, not by hand:

```
jq -c [-n] [--arg k v] -f query.jq input.json > expected.jsonl
```
