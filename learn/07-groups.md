# 7. Groups and sets: `unique`, `group_by`, `-`, `IN`

A list with repeats in it is a common mess, and Windows' `Path` is a good example: installers
add their folder without checking whether it's already there. This page tidies one.

In jqv's REPL `$ENV.Path` is your own Path. Here it is a copy, cut down, in the shape `$ENV`
gives it, so the outputs are the same on every machine: read `.` below as `$ENV`.

Input: [env.json](../testdata/env.json)

## One string, split into entries

The Path is one string with `;` between the folders (`:` on Linux and macOS, where the
variable is `PATH`). `split` makes it an array:

```jq -c
.Path | split(";") | length
```

```json output
22
```

```jq -c
.Path | split(";") | .[:3]
```

```json output
["C:\\Users\\ann\\AppData\\Local\\Microsoft\\WindowsApps","C:\\Python313\\Scripts","C:\\Python313"]
```

The backslashes are doubled because this is JSON: `"C:\\Python313"` is the folder
`C:\Python313`.

## `unique`: each entry once

`unique` drops the repeats. It also **sorts**, so the Path's order, which decides which
program wins, is lost:

```jq -c
.Path | split(";") | unique | length
```

```json output
17
```

```jq -c
.Path | split(";") | unique | .[:4]
```

```json output
["C:\\Program Files (x86)\\oh-my-posh\\bin","C:\\Program Files\\Azure Data Studio\\bin","C:\\Program Files\\Git\\cmd","C:\\Program Files\\Go\\bin"]
```

So the number of repeats is one length taken from the other. `split(";") as $p` names the
array (lesson 5) so it can be used twice:

```jq -c
.Path | split(";") as $p | ($p | length) - ($p | unique | length)
```

```json output
5
```

## `group_by`: equal entries side by side

`group_by(f)` sorts the array by `f` and gathers the elements that give the same `f` into an
array each, so the result is an array of arrays. A group with more than one element is a
repeat:

```jq -c
.Path | split(";") | group_by(.) | map(select(length > 1))
```

```json output
[["C:\\Program Files (x86)\\oh-my-posh\\bin","C:\\Program Files (x86)\\oh-my-posh\\bin"],["C:\\Program Files\\Azure Data Studio\\bin","C:\\Program Files\\Azure Data Studio\\bin"],["C:\\Program Files\\Microsoft SQL Server\\150\\Tools\\Binn","C:\\Program Files\\Microsoft SQL Server\\150\\Tools\\Binn"],["C:\\Users\\ann\\go\\bin","C:\\Users\\ann\\go\\bin"],["C:\\nvm4w\\nodejs","C:\\nvm4w\\nodejs"]]
```

Each group can be summed up as an object, with how many times the entry appears:

```jq -c
.Path | split(";") | group_by(.) | map(select(length > 1) | {entry: .[0], times: length})
```

```json output
[{"entry":"C:\\Program Files (x86)\\oh-my-posh\\bin","times":2},{"entry":"C:\\Program Files\\Azure Data Studio\\bin","times":2},{"entry":"C:\\Program Files\\Microsoft SQL Server\\150\\Tools\\Binn","times":2},{"entry":"C:\\Users\\ann\\go\\bin","times":2},{"entry":"C:\\nvm4w\\nodejs","times":2}]
```

## Equal, ignoring case

Windows doesn't care about case, so `c:\Users\…` and `C:\Users\…` are the same folder, but
`==` (and so `unique` and `group_by(.)`) says they differ. Group by the lower-cased entry
instead:

```jq -c
.Path | split(";") | group_by(ascii_downcase) | map(select(length > 1)) | length
```

```json output
6
```

One more than before. Which group is the new one? The one whose members are not all the same
when compared exactly, so `unique` inside it leaves more than one:

```jq -c
.Path | split(";") | group_by(ascii_downcase) | map(select(unique | length > 1))
```

```json output
[["c:\\Users\\ann\\AppData\\Local\\Programs\\cursor\\resources\\app\\bin","C:\\Users\\ann\\AppData\\Local\\Programs\\cursor\\resources\\app\\bin"]]
```

`unique_by(f)` is `unique` with the same idea: one element for each value of `f`, the first
in each group:

```jq -c
.Path | split(";") | unique_by(ascii_downcase) | length
```

```json output
16
```

## Grouping by something else

`f` can be any filter. Grouping by the folder just under the drive shows where things are
installed; `{(.[0]): length}` builds an object whose key is computed (the parentheses ask for
that), and `add` merges the objects into one:

```jq -c
.Path | split(";") | unique | map(split("\\")[1]) | group_by(.) | map({(.[0]): length}) | add
```

```json output
{"Program Files":5,"Program Files (x86)":1,"ProgramData":1,"Python313":2,"Users":4,"Windows":3,"nvm4w":1}
```

## Repeats, keeping the order

To clean a Path, the first of each entry should stay where it is. `reduce` (lesson 5) walks
the entries and adds each one only if it isn't there yet. `IN(s; x)` says whether `x` is one
of the values `s` gives:

```jq -c
.Path | split(";") | reduce .[] as $e ([]; if IN(.[]; $e) then . else . + [$e] end) | length
```

```json output
17
```

```jq -c
.Path | split(";") | reduce .[] as $e ([]; if IN(.[]; $e) then . else . + [$e] end) | .[:4]
```

```json output
["C:\\Users\\ann\\AppData\\Local\\Microsoft\\WindowsApps","C:\\Python313\\Scripts","C:\\Python313","C:\\Program Files (x86)\\oh-my-posh\\bin"]
```

And where the repeats are, so they can be found and removed by hand. `to_entries` turns an
array into `{key, value}` pairs, whose key is the position:

```jq -c
.Path | split(";") | to_entries | group_by(.value) | map(select(length > 1) | {(.[0].value): map(.key)}) | add
```

```json output
{"C:\\Program Files (x86)\\oh-my-posh\\bin":[3,10],"C:\\Program Files\\Azure Data Studio\\bin":[12,19],"C:\\Program Files\\Microsoft SQL Server\\150\\Tools\\Binn":[11,13],"C:\\Users\\ann\\go\\bin":[17,21],"C:\\nvm4w\\nodejs":[9,18]}
```

## Sets: `-`

On arrays, `a - b` is `a` without every element that is also in `b`. It keeps `a`'s order and
doesn't sort. Which of these folders is missing from the Path?

```jq -c
.Path | split(";") as $p | ["C:\\Program Files\\Go\\bin", "C:\\Program Files\\Git\\cmd", "C:\\Users\\ann\\.cargo\\bin"] - $p
```

```json output
["C:\\Users\\ann\\.cargo\\bin"]
```

And the Path without the Windows folders (every copy of a repeat goes):

```jq -c
.Path | split(";") - ["C:\\Windows", "C:\\Windows\\system32", "C:\\Windows\\System32\\Wbem"] | length
```

```json output
19
```

## Is it there? `IN`, and the trap in `contains`

`IN` with one argument asks whether `.` is one of the values it gives, compared with `==`:

```jq -c
.Path | split(";") as $p | "C:\\Program Files\\Go\\bin", "Go" | IN($p[])
```

```json output
true
false
```

`contains` looks like the same question, but for strings it means **has as a substring**,
and an array contains another when each element of the other is contained in one of its
elements. So `"Go"` is found, inside `C:\Program Files\Go\bin`:

```jq -c
.Path | split(";") | contains(["C:\\Program Files\\Go\\bin"]), contains(["Go"])
```

```json output
true
true
```

Use `contains` to search, `IN` to check membership. `inside` is `contains` the other way
round, `a | inside(b)` being `b | contains(a)`, with the same substring rule: neither of these
is an entry, only part of one, and still the answer is `true`:

```jq -c
.Path | split(";") as $p | ["Python313\\Scr", "nvm4w"] | inside($p)
```

```json output
true
```

Asked with `IN`, entry by entry, `all` says no:

```jq -c
.Path | split(";") as $p | ["Python313\\Scr", "nvm4w"] | all(.[]; IN($p[]))
```

```json output
false
```

To keep the entries that match a word, `select` with `contains` (or `test`, ignoring case)
does it entry by entry:

```jq -c
.Path | split(";") | map(select(test("python"; "i")))
```

```json output
["C:\\Python313\\Scripts","C:\\Python313"]
```

## `INDEX`: an object to look things up in

`INDEX(f)` turns an array into an object whose keys are `f` of each element, the last one
winning where two give the same key. Keyed by the lower-cased entry, the repeats fold away
and a lookup is one `has`:

```jq -c
.Path | split(";") | INDEX(ascii_downcase) | length, has("c:\\program files\\go\\bin")
```

```json output
16
true
```

## In jqv

Try it on your own Path: `jqv repl` on any file, then

```text
$ENV.Path | split(";")
:push
group_by(ascii_downcase) | map(select(length > 1))
```

`:push` keeps the split as a stage, so each line after it starts from the entries. On Linux
the line is `$ENV.PATH | split(":")`.
