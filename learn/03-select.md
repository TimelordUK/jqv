# 3. Isolating records: `select`

`select(cond)` passes its input through when `cond` is true, and gives nothing when it isn't.
That's all of filtering: walk to the records, then `select`.

Input: [people.json](../testdata/people.json)

## Records in one array

The teams are the records:

```jq
.[] | select(.team == "beta")
```

```json output
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
```

`map(select(...))` keeps the answer an array:

```jq -c
map(select(.team == "beta")) | map(.team)
```

```json output
["beta"]
```

## Records deeper down

The people are records too. Walk to them, then select:

```jq -c
.[].people[] | select(.age > 30)
```

```json output
{"name":"Cat","age":41}
{"name":"Dan","age":35}
```

```jq
[.[].people[] | select(.age > 30) | .name]
```

```json output
[
  "Cat",
  "Dan"
]
```

Two conditions in one test, with `and` / `or`:

```jq -c
.[].people[] | select(.age > 26 and .age < 40)
```

```json output
{"name":"Ann","age":30}
{"name":"Dan","age":35}
```

Two `select`s in a row mean the same as `and`. This is how the filter stack layers
(DESIGN §10.1): each layer is one more `select`.

```jq -c
.[].people[] | select(.age > 26) | select(.age < 40)
```

```json output
{"name":"Ann","age":30}
{"name":"Dan","age":35}
```

## Keep the parent, test the child

Teams with anyone over 40. `any(gen; cond)` is true when `cond` holds for any output of
`gen`, and the team comes out once however many match:

```jq -r
.[] | select(any(.people[]; .age > 40)) | .team
```

```text output
beta
```

Compare `select` on the stream directly: a team with two matches would come out twice.

```jq -r
.[] | select(.people[].age > 20) | .team
```

```text output
alpha
alpha
beta
beta
```

## Shaping inside the test

The test can shape a value before comparing it (lesson 2), and the record still comes out
whole:

```jq -c
.[].people[] | select(.name | ascii_downcase | startswith("c"))
```

```json output
{"name":"Cat","age":41}
```

## Missing fields

A missing field is `null`, and `null == "x"` is false, so records without the field go
quietly. An empty output block means nothing came out:

```jq -c
.[].people[] | select(.role == "lead")
```

```json output
```

`null` sorts before every number, so `null > 30` is false too:

```jq -c
.[].people[] | select(.salary > 30)
```

```json output
```

## In jqv

With the cursor on `.[0].people[0].age`, a filter layer starts as `.age` and you add `> 30`.
The engine (Q-015) filters the records of one array. Here the people are spread over two
arrays, one per team, and `.[].people[]` reaches them all. Whether a layer should do that too
is an open question for DESIGN §10.1. This page is where to try the answer out.
