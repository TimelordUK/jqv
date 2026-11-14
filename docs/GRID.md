# jqv — the grid bench (plan, 2026-10-31)

> Lift fields out of a document of repeated records and get a grid. Filter it, sort it, total
> it. Every step is generated jq, recomputed in a split screen.

**Status: plan, nothing built.** Decisions here are recommendations until the owner confirms
them; the ones that need a call are in §12. The queries below, traps included, were run through
gojq 0.12.19 and real jq 1.8.2 against `testdata/` and a generated TeamCity-shaped document, and
gave the same results and the same failures in all 68 cases. The later `teamcity_runs.json` round
found one real difference, `fromdate` (trap 17), and key order differs too (§4).

**Scope.** This is the 9/10ths: grids from JSON of repeated records. An IDE-style editor is
"eventually", and nothing here should be built to prepare for it, beyond keeping the spec and
the jq text separate.

---

## 1. The core idea

The owner's call, which reverses DESIGN §9.4 and §10: **the REPL is read-only.**

```
 keys ──▶ Spec ──▶ compile ──▶ jq text ──▶ gojq ──▶ grid
           ▲                      │
           │                  shown, copyable, never edited
         undo
```

- **The Spec is the state.** A small struct: where the rows are, the columns, filters, group,
  sort, limit. Operators change the Spec; they never touch text.
- **The jq is always derived, always legal, always what ran.** One-way: no parsing jq back.
  `y` in the REPL copies it, so it still teaches jq and still works outside jqv.
- **A bad state can't be typed.** Input is parsed by our own small grammars (§6); a literal
  reaches jq only through JSON encoding, so a stray `"` can't break the query (case M below).
- **Undo is cheap:** a stack of Specs.
- **Later, the door to an IDE:** "detach" copies the jq into a free text editor, leaving the
  operators behind. Not now.

## 2. The rule that decides everything: the grain

Lifting from different levels is the hard part. Without a rule it silently produces wrong
numbers. `{id, qty: .fills[].qty, price: .fills[].price}` on one order with two fills gives
**four** rows, with prices paired to the wrong quantities.

**The grain is what one row of the grid means**: one order, or one fill. It is a single chain
of array steps, such as `.orders[]`, or `.orders[]` then `.fills[]`. Every lifted field falls
into one of four cases:

| Field is | Example (grain `.orders[]`) | What happens |
|---|---|---|
| At the grain, or through objects only | `.id`, `.instrument.symbol` | Plain column: one value per row |
| Above the grain | `.source`, on a grid of orders | Repeats on every row, via `$root` / `$order` |
| Below the grain, through an array | `.fills[].qty` | **Stays in one cell** as an array, until reduced or exploded (§3) |
| Unrelated array at the top | `.trades[].id` | Refused, with a message. That's a join: out of scope |

The **first lift sets the grain** to the deepest array on its path, so lifting from inside a
fill gives a grid of fills. The rule means a row can never multiply by accident: the only way
to get more rows is the explicit explode.

Generalising: on `.orders[3].instrument.symbol` the lift is `.orders[].instrument.symbol`, as
§9.4 already says.

## 3. Sub-arrays: the answer to "what do we do with them?"

Four things, in this order of ease. A below-grain column starts as the first, and one key
moves it on.

**Reduce in the cell** (default). Lossless: you see everything, and no rows change.

```jq
[.orders[] | {id, fills_qty: [.fills[].qty]}]
# → ORD-1001 [1500,1000] · ORD-1002 [] · ORD-1003 [120]
```

A cell operator then picks one of `count` `sum` `min` `max` `first` `last` `join`.
`first`/`last` are the "[index] from each row" idea: `.fills[0].qty`, `.fills[-1].ts`. A missing
index is a clean `null`.

**Explode** (repeat the parent): the expander. Moves the grain one array deeper, and the old
grain's columns become repeats. Outer join, so an order with no fills stays as a row:

```jq
[.orders[] | . as $order | (.fills[]? // null)
 | {id: $order.id, order_qty: $order.qty, fill_qty: .qty, price}]
# 4 rows: ORD-1001 twice, ORD-1002 once with null fill columns, ORD-1003 once
```

The plain `.fills[]` is an inner join: it loses ORD-1002 without a word. On the Nobel fixture
that's 49 prizes with no laureates (1085 rows versus 1036). Inner is a later toggle, not the default.

**Only one chain.** `users_100.json` has `friends` and, inside each, `hobbies`, so that chain can
go deeper; two *sibling* arrays cannot both be exploded (that's the cartesian product again).
The second stays a cell.

**Pick one element by a key** (Q-023). `testdata/fx-trade.json` shows why reduce and explode
aren't enough. `Parties` holds seven parties in no meaningful order, each identified by
`PartyRole`, and what you want is a column per role on the trade's own row. Position is
meaningless, and exploding gives seven rows for one trade:

```jq
{id: .OrderID,
 exec_firm: first((.Parties.NoPartyIDs[] | select(.PartyRole == 1) | .PartyID), null),
 venue:     first((.Parties.NoPartyIDs[] | select(.PartyRole == 64) | .PartyID), null)}
```

On a field inside an element, the pick key takes the sibling that identifies the element
(suggested from the sample: few distinct values, like `PartyRole`) and the value from the cursor's
element. One value per row, so it never changes the grain. Other uses in the same trade:

- **Nested:** `… select(.PartyRole == 1) | .PtysSubGrp.NoPartySubIDs[] | select(.PartySubIDType == 2) | .PartySubID`.
- **By position, where it does mean something:** a swap's near and far legs, `.NoLegs[0]`, `.NoLegs[-1]`.
- **Keyed to the row:** with a grid of legs, each leg's regulatory ID sits in a *sibling* array that
  names the leg. A sibling can't be exploded, but it can be picked, comparing to the row:
  `select(.RegulatoryLegRefID == $leg.LegRefID)`. It's a join within one record, which the
  §2 refusal doesn't need to cover.

An inline tree-style expander inside the grid is left out. Enter on a cell jumping to its source
in the tree does the same job for less (§9).

## 4. Generated jq: the canonical pipeline

Fixed order, one stage per operator. Each stage is optional, so simple grids stay simple.

```
 source │ row filters │ explode │ grain filters │ project │ group │ sort │ limit
```

```jq
# rows
[.orders[] | {id, symbol: .instrument.symbol, qty}]

# a filter and a sort
[.orders[] | select(.qty | numbers | . > 500) | {id, qty}] | sort_by(.qty) | reverse

# something above the row
. as $root | [.orders[] | {source: $root.source, id}]

# a filter at the exploded grain
[.orders[] | . as $order | (.fills[]? // null)
 | select(.qty | numbers | . > 1000) | {id: $order.id, fill_qty: .qty}]

# a filter below the grain: "orders having such a fill"
[.orders[] | select(any(.fills[]?; .qty | numbers | . > 1000)) | {id}]

# group
[.orders[] | {side, qty}] | group_by([.side])
| map({side: .[0].side, n: length, qty: (map(.qty | numbers) | add // 0)})
```

Each filter sits at the level of the field it tests, and need not be a lifted column.

**Rows are objects, for reading.** gojq sorts their keys (`{id, symbol, qty}` comes out as
`id, qty, symbol`; real jq keeps them as written), so **column order comes from the Spec, never
from the output.** A query copied out of jqv gives columns in Spec order under real jq. Column names
are made unique by the Spec too, because a repeated key in an object silently wins
(`{id, id: .x}` has one field). Clashes take the parent's name: `id`, `instrument.id`.

## 5. Traps found by running it

A plausible generator gets each of these wrong, and most fail *silently*.

| # | Naive jq | What goes wrong | Generate instead |
|---|---|---|---|
| 1 | `select(.n < 50)` | jq orders null < numbers < strings: **null and missing pass** | `select(.n \| numbers \| . < 50)` |
| 2 | `select(.n > 50)` | Every string passes (`"n/a"` was selected) | Same guard |
| 3 | `select(.n \| test("x"))` | One number aborts the whole run | `.n \| strings \| test(...)` |
| 4 | `.err.code` | One row where `err` is a string aborts the run | `(try .err.code catch null)` |
| 5 | `{code: .err.code?}` | **The whole row vanishes** | As 4 |
| 6 | `[.id, .err.code?, .ok]` | The cell vanishes and **columns shift left** | As 4 |
| 7 | `.ok? // null` | `//` turns `false` into `null` | As 4 |
| 8 | `add` over a column | Strings concatenate, or a mix aborts | `map(.x \| numbers) \| add // 0` |
| 9 | `add / length`, on no rows | Aborts: cannot divide null by 0 | `if length > 0 then add / length else null end` |
| 10 | `sort_by(-.x)` | `-` errors on a string or null | `sort_by(.x) \| reverse` |
| 11 | **Sum a repeated column** | After exploding, `qty` totals **15300, not 12800**: parents counted once per fill | No total on a repeated column (§7) |
| 12 | `.fills[]` | Parents with no children disappear | `(.fills[]? // null)` |
| 13 | `select((.n \| numbers \| . > 50) \| not)` | A guard yields nothing for a non-number, so negating it **drops** that row, when "not over 50" should keep it | `select(any(.n \| numbers; . > 50) \| not)` |
| 14 | `any(.k[]; .q > 1000)` | Trap 2 again, one level down: a string `q` matches, and a missing `k` aborts | `any(.k[]?; .q \| numbers \| . > 1000)` |

From the shape of the real TeamCity runs (a script's output: a top-level array of flat records):

| # | Naive jq | What goes wrong | Generate instead |
|---|---|---|---|
| 15 | `select(.Number \| numbers \| . > 3000)` | `Number` is a string (`"3299"`), so the guard gives **0 rows, no error** | `select(.Number \| tonumber? \| . > 3000)` |
| 16 | `sort_by(.Number) \| reverse` | Sorted as text: `"99"` beats `"3421"` | `{n: (.Number \| tonumber? // .)}`, then sort |
| 17 | `.Started \| fromdate` | gojq accepts `+00:00`; **real jq aborts**, so the copied query breaks outside jqv | `.Started \| sub("[+]00:00$"; "Z") \| fromdate`. Other offsets are untested |

So the row sample must tell a *number* from a *numeric string*, and a column that is numeric text
is treated as a number by filters and sort. Traps 15–16 are why type comes from the values, not
just the JSON kind. Totals are different: in the Filebeat records `pid` and `threadid` are numeric
text too, and summing an id means nothing, so totals are offered by default only on real JSON
numbers, and on numeric text when asked for.

From the shape of a Filebeat log (separate records, nested objects, no arrays):

| # | Naive jq | What goes wrong | Generate instead |
|---|---|---|---|
| 18 | `{@timestamp, level}` | Doesn't parse: `@` needs quotes | `{"@timestamp", level}`, or `{ts: ."@timestamp"}` |
| 19 | `."@timestamp" \| fromdate` | Fractional seconds abort it, in both engines | `.[:19] + "Z" \| fromdate`, whole seconds only |
| 20 | `{id: (.message \| capture("…").id)}` | **A record with no match vanishes**: trap 5 by another route | `first((… \| capture("…").id), null)`. `try` does not help: no match is empty, not an error |
| 22 | `first(.x[] \| select(…) \| .y, null)` | `,` binds tighter than `\|`, so no match gives nothing and **the row vanishes**. With a match it looks right, so tests on good data pass | Brackets: `first((.x[] \| select(…) \| .y), null)` |
| 21 | `.[]` over several records | jqv's array is one it built, so the copied query needs `jq -s` | The REPL shows `jq -s` when the source was records |

Traps 4–7 only matter where a path really has mixed types, so the `try` is emitted only where
the row sample says so. Clean documents get clean jq, which is the jq worth learning from.

## 6. Filters: `> 50` and friends

On a field, the filter key opens a prompt showing the field, and you type only the condition.
The grammar depends on the field's type, and the prompt stays red until it parses, so nothing
invalid reaches the Spec.

| Type | Typed | Generates |
|---|---|---|
| number | `> 50` `>= 50` `< 50` `<= 50` `= 50` `!= 50` | `.q \| numbers \| . > 50` |
| number | `50..100` | `.q \| numbers \| . >= 50 and . <= 100` |
| string | `vod` | Contains, smartcase, as in search |
| string | `"VOD.L"` | Exact, as quoted search (V-014) |
| string | `~ ^VOD` | `.s \| strings \| test("^VOD")` |
| string | `filled, partial` | `.s \| IN("filled","partial")` |
| any | `!` before any of these | `select(any(.n \| numbers; . > 50) \| not)` |
| bool | `true` `false` | `.b == true` |
| any | `null` `!null` | `.x == null`, `.x != null` |

Smartcase contains is `.s | strings | ascii_downcase | contains("vod")`, with no regex to
escape. An exact-value filter from the cursor (`=` on `"ORD-1002"`) needs no prompt at all.

- Several filters are **AND**. OR is only within one field (`filled, partial`). No brackets.
- Filters are a visible list, each with a key to toggle and remove. Toggling off keeps it in
  the Spec, so "what if I drop that one" is one key.
- Dates (TeamCity's `20261027T091403+0000`, ISO) compare as strings when they sort as strings.
  Fine for now; date-aware filters wait for a real need.

## 7. Grid, sort, totals, group

**Sort goes through jq.** Sorting in Go with `gojq.Compare` would not be faster: 86 ms in Go
versus 118 ms for a full jq re-run on 95,000 rows. So there's no second code path, and the jq
on show is still the whole truth. `s` on a column cycles ascending, descending, off.

**Totals: a footer, also jq.** Per column: `sum` `min` `max` `avg` `count` `distinct`, compiled
as one more query over the rows, so an aggregate never re-runs the extraction:

```jq
[.orders[] | {id, qty}] | {qty_sum: (map(.qty | numbers) | add // 0)}   # 12800
```

**A repeated column shows `—` and says why.** That's trap 11. The total is simply not on offer
until the grain goes back up.

**Group** is a stage over the same rows: mark group columns, then aggregates. It's the
TeamCity summary. On 300 builds, by project and status, with total failed tests:

```jq
[.build[] | {p: .buildType.projectName, s: .status, f: .testOccurrences.failed}]
| group_by([.p, .s])
| map({p: .[0].p, s: .[0].s, n: length, failed: (map(.f | numbers) | add // 0)})
```

Null is a group of its own, and that's correct. Groups come out sorted by key.

**One derived column can't wait: a segment of a delimited field** (Q-021). `Project` is
`… / Scheduled Restart / DEV11`, and the column you'd group on is the last part. Without this,
the everyday "failures per environment" summary can't be built:

```jq
[.[] | {env: (.Project | split(" / ") | last), Status}] | group_by([.env, .Status])
| map({env: .[0].env, Status: .[0].Status, n: length})
```

The delimiter is suggested from the sample, and the segment picked by key. Days work the same
way: `.Started[:10]`.

**Extracting from a message comes after that** (Q-022). Log messages hold real fields:
`capture("publication id '(?<id>[^']+)'").id` pulls out an id that then links records. It needs
regex, so it's a typed prompt with a live preview, not a key. Without it, a filter on the text
still gets most of the way.

**A few rows, many columns: the record view** (V-018). One trade has 36 top-level scalars and
161 in all, which no grid shows well. With few rows the grid turns on its side, a row per column,
as psql's `\x`, and with two or three rows it compares records side by side. A key shows every
scalar in a record without lifting anything:

```jq
[paths(scalars) as $p | {path: ($p | map(tostring) | join(".")), value: getpath($p)}]
```

**Numbers are formatted by the grid.** Untouched values print as written (`0.000053`), but
arithmetic gives `5.3e-05` in real jq, which is no good in a totals row for forward points. The
grid formats them itself (V-019). Exact decimals are out of scope: these are floats.

**Long text in a cell** must be flattened, since one message holds `\r\n\t`:
`gsub("[\r\n\t]+"; " ")` for the display, and the original in a detail pane (T-009).

**Other conditional and computed columns come later** (Q-019), offered as fixed recipes, not as an
expression language. All of these already run:

```jq
((if .side == "buy" then 1 else -1 end) * .qty)                       # signed: total -7200
((.finishDate | strptime("%Y%m%dT%H%M%S%z") | mktime)
 - (.startDate | strptime("%Y%m%dT%H%M%S%z") | mktime))               # build seconds
```

## 8. Design mode: `limit 20`, then commit over everything

The owner's idea. Measured on a 51.7 MB document of 95,000 builds (Ryzen 7950X, gojq on
`doc.Value()`):

| Operation | Whole document | With a limit |
|---|---|---|
| First `Value()`, once, then kept | 218 ms, **+316 MB** | 200 rows: under 1 ms, nothing retained |
| Project 5 columns | 102 ms | 1 ms (`limit(20; …)`) |
| Filter + project | 59 ms | 4 ms even for a rare match |
| Sort | 118 ms | |
| Group, 2 keys + sum | 302 ms | 2 ms (first 200 rows) |
| Explode | 77 ms | |
| `path()` of filtered rows | 53 ms | |

**What that says.** Recomputing the whole document on each *operation* is already fine at
50 MB, and about double at the 100 MB cap. A key press landing in 100–300 ms is acceptable, so
the limit isn't needed for discrete lifts. It earns its place in two other ways:

- **While typing.** A filter prompt re-evaluating on each keystroke needs 1–4 ms, not 300.
- **While deciding.** A 20-row grid is easier to think with, as `TOP 20` is in SSMS.

**Two kinds of limit, and the difference matters:**

| Pipeline has | Limit goes | Example | Result is |
|---|---|---|---|
| No group, no sort | On output | `[limit(20; .orders[] \| select(…) \| {…})]` | The true first 20 matches |
| Group or sort | On source | `[limit(200; .orders[]) \| {…}] \| group_by(…)` | **Partial**: a group of the first 200 rows only |

A partial answer that looks like a real one is the danger here. The second kind must **say so
loudly** in the grid's title (`PREVIEW · first 200 rows`), and totals are marked as partial.

**The flow:** the limit is part of the Spec and appears in the jq, so what you see is still
exactly what ran. `:limit 20` sets it, and the commit key drops it and runs everything, with
any run cancellable. A never-matching filter still has to scan everything (47 ms here), which
is what cancel is for.

## 9. The screen

The split that DESIGN §4 already drew, with the REPL read-only:

```
┌─ jqv  orders.json ────────────────────────────── {4} ┐
│ ▾ orders [3]             │ .orders[] | {id, qty}     │  jq: read-only, always current
│ │ ▾ [0] {9}              │ where: qty > 500   status…│  filters, each toggleable
│ │ │ ● id: "ORD-1001"     ├─ grid · 2 rows · 3 ms ────┤
│ │ │ ▸ instrument {3}     │ id        symbol    qty ▼ │
│ │ │ ● qty: 2500          │ ORD-1002  BARC.L  10000   │
│ │ ▸ [1] {10}             │ ORD-1001  VOD.L    2500   │
│                          │ sum              12500    │
├──────────────────────────┴───────────────────────────┤
│ .orders[].qty      number · 3 of 3 rows · sum 12800  │
└──────────────────────────────────────────────────────┘
```

- `●` marks lifted fields **on every record**, so the tree shows the Spec too.
- Numbers right-aligned, nulls dim, wide columns truncated with the grid scrolling sideways.
- Enter on a grid cell jumps to its source in the tree. `path()` supplies it, through filters
  and explode alike, in 53 ms, without carrying hidden columns in the grid:
  `[path(.orders[] | .fills[] | select(.qty > 500))]` →
  `["orders",0,"fills",0]`, `["orders",0,"fills",1]`.

**Keys (provisional, all currently free):**

| Key | Where | Action |
|---|---|---|
| `tab` | anywhere | Move between tree and grid |
| `a` | tree | Lift this field (again to drop it) |
| `w` | tree, grid | Filter on this field: opens the prompt |
| `=` | tree, grid | Filter to exactly this value |
| `s` | grid | Sort by this column |
| `x` | either | Explode this array / collapse it back into a cell |
| `t` | grid | Cycle the cell or footer aggregate |
| `b` | grid | Group by this column |
| `u` | either | Undo |
| `p` | either | Commit: drop the preview limit and run in full |

`y` copies the jq from the REPL. Exports (CSV, JSON, sql-cli) are V-004, and come straight
from the rows.

## 10. Architecture

```
internal/query   gojq wrapper: compile, run with context + timeout (Q-001)
internal/lens    Spec + Compile(Spec) string. Pure, no TUI, no doc dependency beyond paths
internal/shape   just enough for now: types seen at a path, from a row sample
internal/table   rows + Spec columns → cells, widths, truncation. Replaces the planned V-002 design
```

- Runs go in a `tea.Cmd` with a cancelled context and a generation number, so a stale result
  is dropped. Nothing spawns a process.
- `Value()` is built once, on the first query, and kept. That's about +90% on the tree's memory:
  roughly 1.3 GB at the 100 MB cap. DESIGN §11 saw this coming; it now has a number.
- The Spec serialises to JSON, which gives saved grids (R-008) and recipes almost for free.

**Testing.** The compiler is where trust lives:

- Golden: Spec → exact jq text, one case per stage and each trap in §5.
- Corpus: `examples/jq` cases for the patterns, expected output from real jq.
- **A property test:** random valid Specs over all of `testdata/` must parse, compile and run
  with no error, and with no filter, group or limit they must give exactly one row per grain
  element. That is the "always legal" promise, enforced.
- A mixed-types fixture holding the awkward rows from §5.
- TUI through `Update`, as ever.

## 11. Order of work

Each step leaves something usable, and each adds its wishlist ID to the commit.

0. **Ground truth.** jq 1.8.2 is installed by winget, so §5's queries can join `examples/jq`,
   whose rule is that expected output comes from real jq. Add the mixed fixture.
   **C-004 is now a prerequisite:** jqv refuses a Filebeat document today (`unexpected data
   after the top-level value`), because it is many records with no enclosing array.
   `testdata/teamcity_runs.json` has the real field names (from a screenshot) with invented
   values; real documents can't leave the work PC. The §7 and §8 TeamCity figures came from an
   earlier nested guess, so they stand as timing, not as the real shape. Its failure and running
   rows are guesses too: try jqv on the work PC to see what those really look like.
1. **Q-001** `internal/query`: run with timeout and cancel.
2. **Q-015** `internal/lens`: Spec and compiler, source + project + filter, tested with no UI.
3. **T-025, V-015, T-026:** the split, the read-only REPL, the grid. Lift and drop on one level.
   *First dogfooding stop:* a TeamCity grid on a real document.
4. **Q-016, Q-017:** filter prompt and list, with type guards.
5. **Sort, V-016 totals, Q-018 preview limit.** With these it does the daily job.
6. **Group, with Q-021 segments.** Second stop: the TeamCity summary.
   Filebeat works by here too, with nested columns and grouping by logger, level or minute.
7. **Q-020 explode, Q-023 pick** and cell reductions. Deliberately last of the core: it's the hardest, and
   it should be shaped by use of the single-level grid. The real TeamCity document needs none of
   it, and nor does the Filebeat one, which is good evidence for that order.
8. Later: V-017 jump to source, Q-019 computed columns, R-008 saved grids, V-004 export.

## 12. Decisions needed from the owner

Each has a recommendation; the plan assumes it.

1. **Below-grain lifts: cell or auto-explode?** Lifting `.fills[].qty` onto a grid of orders.
   *Recommend the cell*, then `x`: lifting never changes the row count. The alternative is one
   key shorter, and it changes what every other column means.
2. **First lift's grain: deepest array?** Lifting inside a fill starts a grid of fills.
   *Recommend yes.* It's what the cursor implies, and `x` backs out of it.
3. **Lifting a whole object** (`instrument`): *recommend* lifting its scalar fields, one level.
4. **Sort and limit in the jq, or view-only?** *Recommend in the jq*: Go-side sorting saved almost nothing (§7).
5. **Keyed maps as rows** (USD's `rates`: `to_entries[]`, with key and value columns): real, but
   it can wait. *Recommend after step 6.* The FIX dictionary after C-014 would want it too.
6. **Should a pick's discriminator be suggested or always asked?** *Recommend suggested, with
   one key to change it*, then checked on `fx-trade.json`: `PartyRole` should win.
7. **Does the bar stay read-only for good?** This plan needs it for the 9/10ths. "Detach" (§1)
   is the only proposed way out, and it's later.
