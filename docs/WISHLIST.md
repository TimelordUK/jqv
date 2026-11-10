# jqv — wishlist

Every idea goes here, however wild. We refine them by using the tool.

**IDs:** `C-` core (document model, parsing, performance) · `Q-` query (gojq, errors, AST,
autocomplete) · `V-` views and output (tree, table, raw, export) · `R-` recipes and history ·
`T-` the TUI's layout, keys and chrome. IDs are permanent. Don't renumber; mark items dropped instead.

**Milestones:** `M1` daily-drivable viewer · `M2` query bench · `M3` shape-aware (autocomplete,
schema) · `later` · `?` undecided

**Status:** `idea` → `planned` → `doing` → `done` · `dropped`

**Owner's priorities (2026-10-28):** navigation and finding first, then sensible filtering,
then lifting from the cursor into queries. Rough order: V-007 search → V-010 cursor info →
T-011 jump labels → V-008 structure-aware filter → V-009 finder list → Q-001 onwards, with
Q-006 / Q-013 / Q-014 as the first query features because they start from the cursor.
The navigation layer should feel like vim before the REPL starts (V-007 with T-012, then
T-013–T-015). The REPL's job is to get close quickly: usually a grid of selected fields,
filtered (the order rows for one order id), built by R-005 verbs, not typed out.

---

## Core

| ID | Item | MS | Status |
|---|---|---|---|
| C-001 | Ordered JSON tree from the token stream (key order kept), jq paths with quoting, `Value()` for gojq, fuzzed against encoding/json | M1 | done |
| C-002 | Read a file or piped stdin; keys from the console when stdin is piped | M1 | done |
| C-003 | `--print` / `-d N`: the tree as text, no TUI | M1 | done |
| C-004 | JSON Lines and concatenated values: open as an array of documents | M1 | idea |
| C-005 | Load timing in the status bar (parse ms, node count), like hit's `HIT_TIMING` | M1 | idea |
| C-006 | Incremental row splicing on expand/collapse if rebuilds ever show in timings | later | idea |
| C-007 | Reload on file change (`:e`, or watch); keep open nodes by path | M2 | idea |
| C-008 | Parse errors point at line:col with the surrounding text, and still open what parsed | M1 | idea |
| C-009 | Large-file stance (DESIGN §11): measured 2026-10-29, ~7x memory and ~11 ms per MB. Whole-document loading stays simple; big input goes to lines mode (C-011). Leaner nodes only if limits bite | later | done |
| C-010 | Size guard: `--max-size` (default 100 MB); refuse a big file before reading, stop piped input at the limit, say what to do instead | M1 | done |
| C-011 | Lines mode for big JSON Lines logs: index line offsets, preview the first N, stream jq over lines, matches become a normal document | M2 | idea |
| C-012 | Other formats in (DESIGN §12): translate on load into the same tree, so every key, search and query works unchanged. Pick by extension, then by content. The header shows the translation (`web.config · xml→json`) | M2 | idea |
| C-013 | YAML → JSON (k8s, pipelines, compose). Key order kept; anchors expanded; multi-document files become an array. `doc.ParseYAML`, by extension, fixtures in `testdata/yaml` (DESIGN §12) | M2 | done |
| C-014 | XML → JSON (.NET `web.config` / `app.config`, `.csproj`, NuGet, Maven). Mapping chosen to match an existing tool (yq's is the likely pick) so queries carry over; element order kept. `testdata/FIX4.4.json` is one online converter's answer (attributes as `_name`, repeated elements as arrays): compare against it. It puts attributes last, so a message's name is at the end of its block; ours should put them first. With user guidance, nominate an attribute as the key so a repeated element becomes a keyed map (`TradeCaptureReport: {…}`) rather than an array (owner, 2026-10-30): to try when conversions start | M2 | idea |
| C-015 | `--print-json`: write the translated JSON out, in its key order, so a YAML or XML file can be piped into jq or sql-cli | M2 | done |
| C-016 | Config file (DESIGN §13): `config.toml` in the user config dir, `JQV_CONFIG` to override; a `[set]` table of the same options `:set` changes (`number = true`, `depth = 2`). A bad entry is skipped and reported on the status line, never fatal. `--print` ignores it; `-d` beats it | M1 | done |

## Query

| ID | Item | MS | Status |
|---|---|---|---|
| Q-001 | `internal/query`: gojq parse/compile/run with timeout and cancel; corpus tests (`testdata/queries`) | M2 | doing |
| Q-002 | Query bar: evaluate as you type (debounced), results into the output pane | M2 | planned |
| Q-003 | Errors with position: underline the bad token, a plain-English hint, a suggested fix. In the REPL: the caret, hints for a field asked of an array or null, a function written as a field (`.name.length`), a space after the dot (`. age`), `\(` in a regex, `.` still being what came in after `X as $v |` (`[range(6)] as $i \| map(. + 2)`) | M2 | planned |
| Q-004 | Pipeline stepper: split at top-level `\|`, show the output after each stage, move between stages | M2 | idea |
| Q-005 | AST view (`K`, like nvim LSP hover): the parsed query as a tree. In the REPL as `:tree` (DESIGN §18): each step with what it gives, the first failure with its hint, then `not reached`. Next: open `select(…)`, `map(…)` and operators | M2 | doing |
| Q-006 | Lift the cursor into the query (DESIGN §9.4): on `.orders[3].id` the query starts as `.orders[].id`, the generalised path, not the one element. `y` (exact path) is done; also "filter to this" | M2 | idea |
| Q-007 | Autocomplete paths/keys from the shape index (C-side: `internal/shape`) | M3 | idea |
| Q-008 | The catalogue (DESIGN §16): jq functions as data, with input kinds, what each gives, one line, and an example with its result. Names and arity checked against gojq's `builtins`; examples run through gojq and real jq in a test. Filtering first: tests and shapers for strings and numbers. The help applies each example to the cursor's value | M2 | idea |
| Q-017 | The expression reader (DESIGN §16): a key (`!` proposed) opens a line where `.` is the value under the cursor; jq typed there is evaluated as you type (`trim`, `trim \| length`, `.[:3]`). The base of filter layers, projections and stages | M2 | idea |
| Q-018 | Completion by example (DESIGN §16): after `\|`, run the expression so far on the real value (or a sample of records) and offer the functions that fit what actually came out, each shown applied to it | M2 | idea |
| Q-019 | Lessons (DESIGN §17): `learn/*.md` as a checked notebook: input, `jq` and output blocks, outputs written by `./scripts/learn.ps1` from real jq and checked by `go test` under gojq and jq. Paths, strings and `select` written; each lesson ends with what it means for the reader | M2 | doing |
| Q-009 | `--query` / `-q` non-interactive: run jq with our error hints, print the result | M2 | idea |
| Q-010 | Output key order: gojq sorts keys. Decide whether to re-order output by the source's order where possible | ? | idea |
| Q-011 | "Check with real jq" (if on PATH): run the same query and diff, for gojq differences | later | idea |
| Q-012 | Custom functions available in every query (`flat`, `paths_to(k)`, `table`) | M2 | idea |
| Q-013 | Pick several fields (visual select or mark `id`, `symbol`, `qty` inside one order) → query `.orders[] \| {id, symbol, qty}` → shown as a grid (V-002). A starting point to edit, not a finished query | M2 | idea |
| Q-015 | Filter stack (DESIGN §10.1, owner 2026-11-05): on a field, add a condition (`> 50`) and the tree narrows to the records that pass, each shown whole with its path from the root. Layers AND together, each is plain jq (`.orders[] \| select(.qty > 50)`), the top one can be popped, and the stack can later be lifted into the query bar. The jq form of V-008; comes before projection (Q-013). Engine and corpus done (`internal/filter`, `testdata/filter`); the TUI side is next | M2 | doing |
| Q-016 | Named stages (DESIGN §15): a pipeline as a list of named stages, like SQL's `WITH a AS (…), b AS (…)`, each a jq `def`. Run up to any stage and see its output; change one and the later ones follow. The pipeline is one ordinary jq program | later | idea |
| Q-020 | Scratchpad REPL on the command line (DESIGN §18, owner 2026-11-07): `jqv repl file.json`; each line is jq run on the current `.` and forgotten; `:push` makes it a stage, `:pop` takes it back, `:program` prints the stages as one jq program. `$root` is the whole document, `$_` the last result. The engine (`internal/bench`) is the one the TUI's reader (Q-017) will use | M2 | doing |
| Q-021 | `:help trim` in the REPL, and a browsable list of functions (owner 2026-11-07): what it does, its examples, and `:try N` to run one on its input, `:try N .` on the current `.`. Source: jq's own manual (`manual.yml`, jq-1.8.2, CC BY 3.0) embedded in `internal/catalog`, every example checked under gojq and real jq; gojq differences listed and shown (DESIGN §18). The data side of Q-008 | M2 | done |
| Q-022 | REPL output (DESIGN §18): `:pp` pretty-prints the last result, `:! cmd` pipes it to a program (`bat -l json`, `clip`), a bare `:!` to config.toml's `[repl] pipe`, `:cls` clears the screen. Later: `:set width`, colour | M2 | done |
| Q-023 | Errors point at the manual: `trim cannot be applied to: array` adds `:help trim`, and a hint from its examples (`map(trim)`, `.[] \| trim`) | M2 | idea |
| Q-024 | `keys_unsorted` in jqv: gojq lacks it (`catalog.NotInGojq`), and jq users reach for it to get document order. Define it over the `doc` tree's order, which is the same question as Q-010 | M2 | idea |
| Q-025 | The REPL's own line editor (DESIGN §18, owner 2026-11-07): inline Bubble Tea, so keys reach jqv while typing. Built in pieces: a core with no jq in it (`internal/editor`), and providers plugged in (highlight, hint, complete, check, expand). `ctrl+l` clears, `ctrl+r` searches history (R-001), `F1` on a word prints its `:help` and keeps the line, a dim help line under the input on a known name, tab expands by example (R-006, Q-018). Piped scripts keep the plain reader. Also the TUI reader (Q-017). Core done 2026-11-08: buffer, history, keymap, F1 bound by the REPL; next the Hinter (dim help line) | M2 | doing |
| Q-026 | Multi-line entries in the REPL (DESIGN §18): an unfinished entry (open bracket, trailing `\|`) continues on the next line, `shift+enter` breaks a line; enter runs the whole entry. OPEN: how to join it back onto one line (`ctrl+m`/`ctrl+j` are enter in a terminal) | M2 | idea |
| Q-027 | Colour the REPL's input as you type (DESIGN §18, §19): a jq tokenizer of our own (`internal/jqsyntax`, fuzzed against gojq), then colour that knows things: catalogue functions, names gojq lacks in red, fields not on the document, the line after a parse error underlined, matching brackets. A Highlighter provider on the editor core (Q-025). Comes after Q-031 | M2 | planned |
| Q-028 | Snippets with placeholders in the editor (DESIGN §18): an Expander provider, the way recipes (R-002) reach the line; tab moves between placeholders, and in the TUI the cursor fills them (R-003) | M3 | idea |
| Q-029 | Run to the cursor (DESIGN §18, owner 2026-11-08 from daily use): `F5` runs the line up to the cursor, brackets closed (`[.[].people[] \| select(.age > 30)]`); `ctrl+up`/`ctrl+down` step along the pipeline's stages and run each. `internal/jqsyntax` finds the cut, `internal/assist` binds it. Later: one stage a line with Q-026 | M2 | done |
| Q-030 | `:wq [expr]` in the REPL (DESIGN §18, owner 2026-11-08): quit and hand back the stages and last line as a shell command (`jq '…' file`, pwsh or sh quoting, YAML via `jqv --print-json`), printed and copied; `--emit FILE` for a pwsh function that puts it in PSReadLine's history | M2 | done |
| Q-031 | A total jq lexer (DESIGN §19, agreed 2026-11-08): `jqsyntax.Lex` gives every token with its kind and position and bracket depth (space, comment, string pieces, number, field, variable, format, keyword, name, pipe, operator, bracket, error); the tokens joined give the text back; fuzzed against gojq; half-typed text always lexes. `scan` rebuilt on it. The base of colour (Q-027), tab (R-006), error hints (Q-003) and a later parser for defs and the debugger (Q-032). `Lex` and `scan` on it done 2026-11-09 | M2 | done |
| Q-032 | A jq script workbench (owner 2026-11-08, end goal; first filed as Q-031 in ccccf08, renumbered at the merge because the lexer took Q-031 on the other machine): load a `.jq` file (`examples/jq/team-report`) beside the document; its `def`s become available on every line; run any chunk (a def, a named set, the pipeline up to a point); a debug mode that steps through it showing what each part gives (`:tree` and F5/ctrl+up/down grown up), with IntelliSense and highlighting. Builds on `jqsyntax` (UpTo/Stops, and the lexer Q-031), `:tree`, the catalog, Q-012, Q-016, Q-025–Q-027, and the TUI reader (Q-017). OPEN: how a script maps onto the REPL (stages? defs as a prelude?), and stepping into a `def` | later | idea |
| Q-033 | Step inside a bracket (DESIGN §19, owner 2026-11-09 from daily use): ctrl+up/down stop inside `[…]` but always run it closed, so `[.[].people[] \| select(…)]` only ever shows the collected array, never the stream inside. Stepping in should run the stage the bracket is in, then its inside as a stream: `[A \| B]` → `A \| B`, `map(f)` → `… \| .[] \| f`, `{k: [X]}` → `X`, `select(f)` → `f` on the stage's input; a key to go in and out. Needs a bracket tree over the lexer's tokens (Q-031), not a full parser | M2 | idea |
| Q-034 | `:lex [expr]` in the REPL (DESIGN §19, owner 2026-11-10): the lexer's tokens for the last line or expr, as TestLex writes them, then the brackets with their depths and any errors with their positions. Beside `:tree`; half-typed lines are fine. For checking what colour and stepping are built on, and turning a misread line into a test | M2 | done |
| Q-014 | Aggregate lift: on a number inside an array of objects (`qty` in `.orders[]`), show sum/min/max/avg/count straight away, and lift `[.orders[].qty] \| add` into the query. Later: small inline operations | M2 | idea |

## Views and output

| ID | Item | MS | Status |
|---|---|---|---|
| V-001 | Output pane as a tree (same widget as the document) | M2 | idea |
| V-002 | Table mode: array of objects → columns from the union of keys, nested values summarised | M2 | idea |
| V-003 | Raw mode: pretty-printed JSON with syntax colour | M2 | idea |
| V-004 | Export output: JSON / JSONL / CSV to a file or clipboard; hand-off to sql-cli | M2 | idea |
| V-005 | Schema view: the shape index shown as a tree (key, types seen, how often, example) | M3 | idea |
| V-006 | Collapsed containers show a one-line preview (`{id: "ORD-1001", status: …}`) | M1 | idea |
| V-007 | Vim search (DESIGN §9.1): `/` forward, `?` back, `n`/`N` repeat, wrap with vim's messages; keys and values; smartcase with `\c`/`\C`; opens closed parents and centres an off-screen match; `match 3/17` in the status bar; matches highlighted; esc clears. Later: search history | M1 | done |
| V-008 | Structure-aware filter (DESIGN §9.2): hide what doesn't match but keep each match's ancestors (so you see where it lives) and a matching container's whole subtree (TeamCity: `elastic` keeps the projects nested under it). Fixture from a real TeamCity response | M1 | idea |
| V-009 | Telescope-style finder (DESIGN §9.3): `f`, type a phrase, get a scrollable list of hits under the tree (parent path + the row itself); moving through the list moves the tree, opening each hit and closing it again; enter lands, esc goes back. It is the last search afterwards, so `n` `N` `z/` carry on, and `F` reopens it, or any `/` search, as a list, as vim's quickfix does | M1 | done |
| V-010 | Cursor info (DESIGN §9.4): beside the path, the generalised path (`.orders[].id`), type, position among siblings, and for a field in an array of objects how many elements have it, distinct count, sample values, min/max for numbers | M1 | idea |
| V-011 | Fold to matches (`z/`, owner's idea 2026-10-29): collapse everything except the paths down to each match of the last search, cursor on the first. Unlike the filter (V-008) nothing is hidden: non-matching siblings stay as one closed line each | M1 | done |
| V-013 | Incremental search (DESIGN §9.1, owner 2026-10-31): the cursor moves as the pattern is typed, with the count beside it; esc restores the cursor, the scroll, the open nodes and the previous search | M1 | done |
| V-014 | Quoted search (DESIGN §9.1, owner 2026-10-31): `/"AE"` matches a whole key or string, not part of one; `"AE`, before the closing quote, matches the start. Works in `/`, `?` and the finder | M1 | done |
| V-015 | Compact rows (DESIGN §14, owner 2026-10-30): a small container of scalars shows inline on one row (`[0] {_name: "BeginString", _required: "Y"}`), so long arrays of little records like the FIX `field` lists take one row each. `l` lifts one out, `h` puts it back; `zi` toggles a whole region; `:set compact` and the config make it the default | M1 | idea |
| V-016 | `:w file.json` / `:w! file.json` (overwrite): write what is on show, which under a filter stack (Q-015) is the document with only the kept records, in the source's key order | M2 | idea |
| V-012 | Fold to a jq expression: the same as `z/` but the matches are what a jq expression selects (`paths(.. \| strings \| test("olivia";"i"))`, `.[] \| select(.age > 90)`), so structure-aware conditions can drive what opens | M2 | idea |

## Recipes and history

| ID | Item | MS | Status |
|---|---|---|---|
| R-001 | Query history, persisted, recalled with up/down and a finder (hit-style). Kept in hit's JSONL format with hit's packages (hit C-044), in a file jqv owns; failed lines kept but not recalled; this document's first (DESIGN §18). Done 2026-11-08: recording and up-arrow. Next: `ctrl+r` ranked by hit's search, then the TUI | M2 | doing |
| R-002 | Recipes: named, parameterised jq snippets in TOML ("group by X and count", "find key K anywhere") | M2 | idea |
| R-003 | Recipe picker that fills parameters from the cursor node (a key, a path) | M3 | idea |
| R-004 | Save a session: document path, open nodes, query, output mode | later | idea |
| R-005 | **The everyday 90%** as a few verbs, so they are never hand-typed: pick fields → grid, filter where field = value (on `"ORD-1002"`: `.orders[] \| select(.id == "ORD-1002")`), sort by, group and count, flatten nested to columns. Each verb fills from the cursor and writes plain jq into the bar, so it can be edited and chained (DESIGN §10) | M2 | idea |
| R-007 | Follow a tree of ids (DESIGN §11): on an id, collect every message linked to it through parent fields, in one streaming pass. FlexTrade drop copy (gRPC messages as JSON lines): children name their parent, never the root | M2 | idea |
| R-008 | Saved stages as jq modules (DESIGN §15): "lock it in" writes a stage as a `def` in a module file (`import "jqv/trading" as t;`), so recipes are reusable components inside jqv and run with plain jq outside it. May replace R-002's TOML snippets | later | idea |
| R-006 | Contextual completion in the query bar: after `.orders[].` offer the keys seen there; after `select(.status == ` offer the values seen; verbs (R-005) offered beside builtins | M3 | idea |

## TUI

| ID | Item | MS | Status |
|---|---|---|---|
| T-001 | Tree view: vim keys, open/close, zo/zc/zR/zM/z1–z9, gg/G, half pages, indent guides, colour by type | M1 | done |
| T-002 | Copy the path (`y`) and the value (`Y`) of the cursor node | M1 | done |
| T-003 | Split layout: document · query · output, resizable, focus moves with a key | M2 | idea |
| T-004 | Help overlay (`?`) from the keymap table in `tui/keys.go`; a test fails if a listed key does nothing | M1 | done |
| T-005 | Keymap as data in config.toml | later | idea |
| T-006 | Mouse: click to open/close, wheel to scroll | later | idea |
| T-007 | OSC 52 copy for SSH sessions | later | idea |
| T-008 | Themes (the palette is one table today) | later | idea |
| T-009 | Long strings: wrap or show in a detail pane rather than only truncate | M1 | idea |
| T-010 | Single-key bulk open/close from the cursor: `L` opens the whole subtree, `H` closes everything below but keeps the node open (outline). `zo`/`zc` stay | M1 | done |
| T-011 | Jump labels, like flash.nvim / leap / hop (DESIGN §9.1): press the jump key, type one or two characters, every visible match gets a short label, press the label to land. Reaches anything on screen in 3–4 keys | M1 | idea |
| T-012 | `?` is for backward search (V-007), so help moves to `F1` (owner, 2026-10-28: a natural vim environment) | M1 | done |
| T-013 | Re-centre, vim style: `zz` cursor to middle, `zt` top, `zb` bottom | M1 | done |
| T-014 | Row numbers in a gutter: `:set nu`, `:set rnu` (relative; with both, the cursor row is absolute as in vim), `:set nonu`. Numbers are rows on show, not file lines | M1 | done |
| T-015 | Counts before motions (`5j`, `3n`), so relative numbers pay off | M1 | idea |
| T-016 | Full pages: `ctrl+f` / `ctrl+b` and PgDn/PgUp move a screen less two rows of context; `ctrl+d` / `ctrl+u` stay half pages | M1 | done |
| T-017 | `ctrl+g`: file, row n of m with vim's `--n%--`, how far through the whole document (stable as nodes open and close), depth | M1 | done |
| T-018 | `:` command line, a small vim subset: `:123` go to row, `:set nu/rnu/nonu/nornu/nu!`, `:q`. Unknown commands say so. Grows only when a command earns it (`:e`, `:w out.json` later) | M1 | done |
| T-019 | `[{` `]}`: start / end of the enclosing object, as vim. A search hit on a field at the tail of a record gets to the record's top without closing anything | M1 | done |
| T-020 | `%`: start of a block to its last row and back. Closing brackets are not rows, so the last row stands in for one, and the way back is remembered because one row can end several blocks | M1 | done |
| T-021 | `{` `}`: step through the blocks at this level (one FIX message to the next), opening each on landing and putting it at the top of the screen | M1 | done |
| T-022 | `(` `)`: iterate the array the cursor is in, element by element, from any depth inside the element (Nobel prizes, or the laureates of one). Unlike `{` `}` it never moves across object keys | M1 | done |
| T-024 | The help overlay scrolls: it is 40 rows and growing, so on a shorter terminal the end is cut off with no way to reach it | M1 | idea |
| T-023 | Scroll lock (`zl`): the cursor stays on the middle row and the document moves under it, as vim's `scrolloff=999`. `locked` in the status bar while on | M1 | done |
| T-025 | Options as one table (DESIGN §13): name, alias, type, default, help. `:set` takes vim's forms over it (`nu`, `nonu`, `nu!`, `x=60`, `nu?`, bare `:set`); F1 and the config file (C-016) read the same table | M1 | done |
| T-026 | `Y` on a string copies its text, without quotes: it is nearly always pasted into something else (owner, 2026-11-05, from use at work). Containers and other scalars stay JSON | M1 | done |
