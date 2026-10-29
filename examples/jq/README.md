# jq samples

Small, real jq queries worth keeping: each folder holds the query, an input, and the output
real jq gives. `go test ./examples/...` runs every one through gojq (the engine jqv embeds)
and fails if the output changes, so they stay working and show any gojq/jq difference.
These are the seed for jqv's recipes (R-002, R-005).

| Sample | What it shows |
|---|---|
| [follow-order-tree](follow-order-tree/query.jq) | Every message in one order's tree, from a log where children name only their parent. One streaming pass, holding only the id set (DESIGN §11, R-007) |
| [order-rows](order-rows/query.jq) | Pick fields, flatten a nested object, filter: rows ready for a grid |
| [sum-filled-qty](sum-filled-qty/query.jq) | Sum a nested array per item and in total, with `// 0` for empty arrays |

**Adding one:** make a folder with `query.jq` (a comment at the top: what it's for and the
command line), `input.json` or `input.jsonl`, and `case.json` if it needs `--arg`s or `-n`:

```json
{"args": {"root": "P1"}, "null_input": true}
```

Then produce the expected output **with real jq**, not by hand:

```
jq -c [-n] [--arg k v] -f query.jq input.json > expected.jsonl
```
