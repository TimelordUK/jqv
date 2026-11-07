# 4. Arrays: one, some, or each

Lesson 1 used `.[0]` and `.[]`. There is a third way into an array, the slice `.[a:b]`, and
the three give back different kinds of thing:

| You write | You get                          |
|-----------|----------------------------------|
| `.[n]`    | one element                      |
| `.[a:b]`  | an **array** of some elements    |
| `.[]`     | each element, one output at a time |

Most surprises with arrays come from forgetting which one you're holding.

Input: [people.json](../testdata/people.json)

## Index: one element

`.[1]` is the second team, an object, so `.people` works straight on it:

```jq -c
.[1].people[0]
```

```json output
{"name":"Cat","age":41}
```

## Slice: an array of some elements

`.[1:]` is "from index 1 to the end". It drops the first team, but what comes back is still an
array, here with one team in it:

```jq -c
.[1:]
```

```json output
[{"team":"beta","people":[{"name":"Cat","age":41},{"name":"Dan","age":35}]}]
```

So asking it for `.people` is asking an array for a field, the same error as lesson 1:

```jq
.[1:].people[0]
```

```text error
jq: error (at <stdin>:3): Cannot index array with string ("people")
```

Add `[]` after the slice to step into the elements it kept, then carry on as before:

```jq -c
.[1:][].people[0]
```

```json output
{"name":"Cat","age":41}
```

That is the same as writing the steps out with `|`:

```jq -c
.[1:] | .[] | .people[0]
```

```json output
{"name":"Cat","age":41}
```

## Slice bounds

`.[a:b]` takes from `a` up to, but not including, `b`. Leave out either end to run to the edge.
On the people of the first team:

```jq -c
.[0].people[0:1]
```

```json output
[{"name":"Ann","age":30}]
```

```jq -c
.[0].people[1:]
```

```json output
[{"name":"Bob","age":25}]
```

```jq -c
.[0].people[:1]
```

```json output
[{"name":"Ann","age":30}]
```

Negative bounds count from the end, so `.[:-1]` drops the last element:

```jq -c
.[:-1][].team
```

```json output
"alpha"
```

A slice past the end is an empty array, not an error, and an index past the end is `null`:

```jq -c
.[5:]
```

```json output
[]
```

```jq -c
.[5]
```

```json output
null
```

## Slicing the inner arrays

Put the slice where the array is. Here it's each team's `people`, so it goes after `.people`:
every team, everyone but the first person.

```jq -c
.[].people[1:]
```

```json output
[{"name":"Bob","age":25}]
[{"name":"Dan","age":35}]
```

That gives one array per team. Add `[]` again to get the people themselves:

```jq -c
.[].people[1:][]
```

```json output
{"name":"Bob","age":25}
{"name":"Dan","age":35}
```

```jq -c
.[].people[1:][].name
```

```json output
"Bob"
"Dan"
```

## A slice is just a smaller array

Anything that works on an array works on a slice: `length`, `map`, collecting with `[ ]`.

```jq
.[1:] | length
```

```json output
1
```

```jq -c
.[1:] | map(.team)
```

```json output
["beta"]
```

```jq -c
[.[].people[:1][].name]
```

```json output
["Ann","Cat"]
```

## Generating arrays: `range`

`range` makes numbers instead of reading them. Like `.[]`, it is a generator: it gives one
output per number. These queries need no input, so they run with `-n`.

`range(n)` counts from 0 up to, but not including, `n`:

```jq -n
range(3)
```

```json output
0
1
2
```

`range(from; upto)` starts at `from`. As with slices, `upto` itself isn't included, so 1 to 10 is
`range(1; 11)`. Wrap it in `[ ]` to collect the numbers into an array:

```jq -nc
[range(1; 11)]
```

```json output
[1,2,3,4,5,6,7,8,9,10]
```

Arguments are separated by `;`. A `,` means "both outputs", so `range(1, 4)` is `range(1)` and
then `range(4)`, two counts one after the other:

```jq -nc
[range(1, 4)]
```

```json output
[0,0,1,2,3]
```

A third argument is the step:

```jq -nc
[range(1; 11; 2)]
```

```json output
[1,3,5,7,9]
```

The same odd numbers, picked with `select` (lesson 3) instead of a step:

```jq -nc
[range(1; 11) | select(. % 2 == 1)]
```

```json output
[1,3,5,7,9]
```

### Every other element of any array

`range` can make indexes too. `.[range(0; length; 2)]` looks up indexes 0, 2, 4…, so it picks
every other element by position, whatever the elements are:

```jq -nc
[range(1; 11)] | [.[range(0; length; 2)]]
```

```json output
[1,3,5,7,9]
```

Start at 1 for the elements at odd indexes:

```jq -nc
[range(1; 11)] | [.[range(1; length; 2)]]
```

```json output
[2,4,6,8,10]
```

On the teams, every other person counting from the first:

```jq -c
[.[].people[]] | [.[range(0; length; 2)].name]
```

```json output
["Ann","Cat"]
```

## In jqv

In the reader, the hint is the error: "Cannot index array with string" right after a slice
means the slice worked and needs a `[]` before the next field. With the cursor on a `people`
array, `.` is that array, so the slices above are just `.[1:]`, `.[:1]` and so on.
