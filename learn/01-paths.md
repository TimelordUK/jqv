# 1. Paths: getting to a value

Every jq program is a filter: a value goes in, values come out. `.` is the value going in.
Everything in this lesson is a way of walking from `.` to something inside it.

The input for this lesson:

Input: [people.json](../testdata/people.json)

## The whole thing

```jq
.
```

```json output
[
  {
    "team": "alpha",
    "people": [
      {
        "name": "Ann",
        "age": 30
      },
      {
        "name": "Bob",
        "age": 25
      }
    ]
  },
  {
    "team": "beta",
    "people": [
      {
        "name": "Cat",
        "age": 41
      },
      {
        "name": "Dan",
        "age": 35
      }
    ]
  }
]
```

## One element, one field

`.[0]` is the first element of an array; `.team` is a field of an object. They chain.

```jq
.[0].team
```

```json output
"alpha"
```

```jq
.[0].people[1].name
```

```json output
"Bob"
```

Negative indexes count from the end:

```jq
.[-1].team
```

```json output
"beta"
```

A field that isn't there is `null`, not an error. This is why a filter on a missing field
drops a record quietly (see lesson 3):

```jq
.[0].colour
```

```json output
null
```

But asking an array for a field is an error. jq says what it was asked to index:

```jq
.team
```

```text error
jq: error (at <stdin>:3): Cannot index array with string ("team")
```

## Every element: `.[]`

`.[]` gives each element as a separate output. That's what makes jq a stream: one value in,
several out.

```jq
.[].team
```

```json output
"alpha"
"beta"
```

`.[]` again goes one level further, into each team's people:

```jq
.[].people[].name
```

```json output
"Ann"
"Bob"
"Cat"
"Dan"
```

## `|`: feed each output into the next filter

`a | b` runs `b` on each output of `a`. `.[].team` is shorthand for `.[] | .team`.

```jq
.[] | .team
```

```json output
"alpha"
"beta"
```

The right side of `|` sees one value at a time, so `.` there means "this element":

```jq
.[] | .people | length
```

```json output
2
2
```

## Collecting a stream back into an array: `[ ]`

```jq
[.[].people[].name]
```

```json output
[
  "Ann",
  "Bob",
  "Cat",
  "Dan"
]
```

`map(f)` is exactly `[.[] | f]`:

```jq
map(.team)
```

```json output
[
  "alpha",
  "beta"
]
```

## Building objects

`{team}` is short for `{team: .team}`. The values can be any filter, so here `names` collects
each team's names:

```jq
map({team, names: [.people[].name]})
```

```json output
[
  {
    "team": "alpha",
    "names": [
      "Ann",
      "Bob"
    ]
  },
  {
    "team": "beta",
    "names": [
      "Cat",
      "Dan"
    ]
  }
]
```

`-c` prints each output on one line, as it does on the command line:

```jq -c
.[] | {team, size: (.people | length)}
```

```json output
{"team":"alpha","size":2}
{"team":"beta","size":2}
```

## In jqv

Each query above is what you'd type in the reader (DESIGN §16) with the cursor on the root.
With the cursor on `.[0]` instead, `.` *is* that team, so the same names are `[.people[].name]`.
The reader's job is to show the result as you type, as this page does.
