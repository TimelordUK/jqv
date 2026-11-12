# One FIX message's definition as an indented outline, from fix-dictionary's output: each
# field with its tag and type, components and groups opened in place, `*` for required.
#
#   jq -f ../fix-dictionary/query.jq FIX4.4.json | jq -r --arg type p -f query.jq
#
# The two programs compose: fix-dictionary does the resolving once, and this only reads it.

def pad($n): tostring | ([range($n - length) | " "] | join("")) + .;
def indent($d): [range($d) | "  "] | join("");
def req: if .required then " *" else "" end;

def show($d):
  .[]
  | if .kind == "field" then "\(.tag | pad(5))  \(indent($d))\(.name)  \(.type)\(req)"
    elif .kind == "group" then "\(.tag | pad(5))  \(indent($d))\(.name)  group\(req)", (.fields | show($d + 1))
    else "       \(indent($d))\(.name)  component\(req)", (.fields | show($d + 1))
    end;

.messages[$type] // error("no message with MsgType \($type)")
| "\(.msgtype)  \(.name)  (\(.category))", (.fields | show(1))
