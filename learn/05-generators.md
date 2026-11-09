# 5. Generators: many outputs, joined and nested

A jq filter can give any number of outputs: none (`empty`), one (`.name`), or many (`.[]`,
`range`). A filter that gives many is a **generator**, and how generators combine is most of
what makes jq feel strange at first. Three rules cover it:

| You write  | What happens                                            |
|------------|---------------------------------------------------------|
| `a, b`     | **joined**: every output of `a`, then every output of `b` |
| `a \| b`   | **nested**: `b` runs once for each output of `a`          |
| `a + b`    | **paired**: every combination of the two (careful: see below) |

Lesson 4 introduced `range`. Here it's the generator to experiment with, since it needs no input.

## `,` joins streams

`,` gives the outputs on the left, then the ones on the right. They are separate outputs,
not an array:

```jq -nc
range(3), range(10; 12)
```

```json output
0
1
2
10
11
```

Wrap the whole thing in `[ ]` to collect them into one array:

```jq -nc
[range(3), range(10; 12)]
```

```json output
[0,1,2,10,11]
```

Which is the same as adding two arrays:

```jq -nc
[range(3)] + [range(10; 12)]
```

```json output
[0,1,2,10,11]
```

The two sides of `,` don't have to match. Any filters will do:

```jq -nc
1, "two", [3], {four: 4}
```

```json output
1
"two"
[3]
{"four":4}
```

On a document, `,` picks several things at once:

Input: [people.json](../testdata/people.json)

```jq -c
.[0].team, .[1].people[0].name
```

```json output
"alpha"
"Cat"
```

## Counting down, then up and down

A negative step counts down. The end is still left out, so `range(3; -1; -1)` is 3 to 0:

```jq -nc
[range(3; -1; -1)]
```

```json output
[3,2,1,0]
```

Join an up count and a down count with `,` to go up and back. The down count starts at 2,
not 3, so the peak isn't repeated:

```jq -nc
[range(4), range(2; -1; -1)]
```

```json output
[0,1,2,3,2,1,0]
```

Then feed each number into something. Here each number becomes a power of 10:

```jq -nc
(range(4), range(2; -1; -1)) | pow(10; .)
```

```json output
1
10
100
1000
100
10
1
```

The parentheses there are only for the reader. `|` binds more loosely than anything else in
jq, so `a, b | f` already means `(a, b) | f`, and without them the result is the same:

```jq -nc
range(4), range(2; -1; -1) | pow(10; .)
```

```json output
1
10
100
1000
100
10
1
```

Where parentheses do matter is an operator that binds *tighter* than `,`, such as `+` or `*`.
Here `* 10` only takes the second range:

```jq -nc
[range(3), range(10; 12) * 10]
```

```json output
[0,1,2,100,110]
```

With parentheses it takes both:

```jq -nc
[(range(3), range(10; 12)) * 10]
```

```json output
[0,10,20,100,110]
```

From loosest to tightest: `|`, `,`, `//`, `or`, `and`, comparisons, `+ -`, `* / %`.
When in doubt, add the parentheses: they never hurt.

The same up-and-down as an array, by mirroring: everything but the last element, reversed,
added on the end (slices from lesson 4):

```jq -nc
[range(4)] | . + (.[:-1] | reverse) | map(pow(10; .))
```

```json output
[1,10,100,1000,100,10,1]
```

## `|` replaces `.`; `as $x` keeps it

After `range(5) |`, `.` is the number. So a filter that doesn't mention `.` ignores it: this
gives `"ee"` five times.

```jq -nc
range(5) | "greetings"[2:4]
```

```json output
"ee"
"ee"
"ee"
"ee"
"ee"
```

Use `.` to use the number:

```jq -nc
range(4) | "greetings"[.:.+2]
```

```json output
"gr"
"re"
"ee"
"et"
```

But often you need the number **and** the value you started with. `range(…) as $i | …` gives
each number a name and leaves `.` alone:

```jq -nc
"greetings" | [range(0; length) as $i | .[$i:]]
```

```json output
["greetings","reetings","eetings","etings","tings","ings","ngs","gs","s"]
```

Here `length` and `.[$i:]` both see the string, while `$i` counts. Chunks of three:

```jq -nc
"greetings" | range(0; length; 3) as $i | .[$i:$i+3]
```

```json output
"gre"
"eti"
"ngs"
```

On the document, every prefix of the first team's names:

```jq -c
[.[0].people[] | .name | range(0; length) as $i | .[:$i+1]]
```

```json output
["A","An","Ann","B","Bo","Bob"]
```

## Nested generators: every combination

Two `as` bindings in a row are nested loops. For each `$a`, every `$b` runs:

```jq -nc
[range(2) as $a | range(3) as $b | [$a, $b]]
```

```json output
[[0,0],[0,1],[0,2],[1,0],[1,1],[1,2]]
```

Every team paired with every person (a cross join):

```jq -c
[.[].team as $t | .[].people[].name as $n | "\($t):\($n)"]
```

```json output
["alpha:Ann","alpha:Bob","alpha:Cat","alpha:Dan","beta:Ann","beta:Bob","beta:Cat","beta:Dan"]
```

## The surprise: generators inside an operator

An operator between two generators also gives every combination, but **the right side is the
outer loop**. jq evaluates the right operand first, so for each value of `range(3)` it runs
through all of `range(2) * 10`:

```jq -nc
[range(2) * 10 + range(3)]
```

```json output
[0,10,1,11,2,12]
```

The same 6 numbers in left-to-right order need the `as` form, which always reads as written:

```jq -nc
[range(2) as $a | range(3) as $b | $a * 10 + $b]
```

```json output
[0,1,2,10,11,12]
```

The rule of thumb: if the order of the combinations matters, name them with `as`.

## Counting and totalling a stream

To count a stream, collect it and take the `length`. To total it without building an array,
`reduce` folds the stream into one value, here the sum of the powers of 10:

```jq -nc
[range(4), range(2; -1; -1)] | length
```

```json output
7
```

```jq -nc
reduce (range(4), range(2; -1; -1)) as $n (0; . + pow(10; $n))
```

```json output
1222
```

`first`, `last` and `limit` take part of a stream:

```jq -nc
first(range(10; 20)), last(range(10; 20)), [limit(3; range(10; 20))]
```

```json output
10
19
[10,11,12]
```

## In jqv

The REPL shows the count under a generator's outputs (`(7 outputs)`), and `:tree` shows the
count at each step, which is where a stream grows (`.[]`, `range`) or shrinks (`select`).
`,` and `as` are single steps in `:tree` for now: opening them, to show each side's outputs
and the value bound to `$i`, is the next piece of Q-005.
