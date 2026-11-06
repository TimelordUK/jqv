# 2. Strings: shaping a value before you test it

Most filters end in a test on a string: is it this, does it contain that. Real data is
untidy, so the test often needs the string shaped first: trimmed, lower-cased, cut down.

The input here is a single string. `-r` prints a string as its text, without quotes.

```json
"  Hello World  "
```

## Shaping

```jq
trim
```

```json output
"Hello World"
```

```jq
trim | length
```

```json output
11
```

```jq
length
```

```json output
15
```

The order matters. `.[:5]` takes the first five characters, and here two of them are spaces:

```jq
.[:5]
```

```json output
"  Hel"
```

```jq
trim | .[:5]
```

```json output
"Hello"
```

```jq -r
trim | ascii_downcase
```

```text output
hello world
```

```jq
trim | split(" ")
```

```json output
[
  "Hello",
  "World"
]
```

`ltrimstr` takes a prefix off when it's there, and leaves the string alone when it isn't:

```jq -r
trim | ltrimstr("Hello ")
```

```text output
World
```

## Testing

A test gives `true` or `false`. In a filter layer, `true` keeps the record.

```jq
trim == "Hello World"
```

```json output
true
```

`==` is exact. Case counts:

```jq
trim == "hello world"
```

```json output
false
```

```jq
contains("World")
```

```json output
true
```

```jq
trim | startswith("Hello")
```

```json output
true
```

`test` takes a regular expression, and `"i"` ignores case:

```jq
test("^\\s*hello"; "i")
```

```json output
true
```

## Wrong type, wrong answer

String functions on a number are errors, not `false`:

```json
42
```

```jq
trim
```

```text error
jq: error (at <stdin>:1): trim input must be a string
```

`tostring` first makes it a string:

```jq
tostring | length
```

```json output
2
```

And `null`, which is what a missing field gives, has length 0 but can't be trimmed:

```json
null
```

```jq
length
```

```json output
0
```

```jq
trim
```

```text error
jq: error (at <stdin>:1): trim input must be a string
```

## In jqv

On a string, the reader's completion offers the functions that fit a string (DESIGN §16), each
shown applied to the value under the cursor, as above: `trim → "Hello World"`.
