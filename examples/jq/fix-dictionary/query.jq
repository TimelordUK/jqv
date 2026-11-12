# A FIX dictionary (QuickFIX's XML, run through an XML→JSON converter: testdata/FIX4.4.json)
# turned into what an application wants: fields by name and by tag, and each message with
# its references resolved, so a component's fields and a group's members sit where they
# are used.
#
#   jq -f query.jq FIX4.4.json | jq '.messages.AE.fields[0]'
#   jq -f query.jq FIX4.4.json | jq '.messages.p.fields[] | select(.kind == "component")'
#
# Two things about the converted JSON shape this program (`:structure` shows both):
#  - A lone child comes out as an object, not an array of one, so every list goes through
#    `items` before it is walked.
#  - The converter gathered children by element name (field[], component[], group), so the
#    XML's order across kinds is lost. A message lists its fields, then its components,
#    then its groups; within each kind the XML's order is kept.
# The whole dictionary, saved once and then queried:
#
# jq -f examples/jq/fix-dictionary/query.jq testdata/FIX4.4.json > $env:TEMP/fix44.json
#
# jq '.messages.AE.fields[0]' $env:TEMP/fix44.json
# jq '.fields.PartyRole.values' $env:TEMP/fix44.json
# jq '.tags["35"]' $env:TEMP/fix44.json
# jq -r --arg type D -f examples/jq/fix-message/query.jq $env:TEMP/fix44.json

# A list, whatever the converter made of it: none, one (an object) or several (an array).
def items: if . == null then [] elif type == "array" then . else [.] end;

def required: ._required == "Y";

.fix as $fix

# Every field by name: its tag as a number, type, and the enum values when it has any.
| ($fix.fields.field | items
   | map({
       key: ._name,
       value: ({tag: (._number | tonumber), name: ._name, type: ._type}
         + if .value then {values: (.value | items | map({key: ._enum, value: ._description}) | from_entries)} else {} end)
     })
   | from_entries) as $fields

| ($fix.components.component | items | INDEX(._name)) as $components

# The members of a message, component or group, resolved: a field gets its definition (but
# not its enum values, which would be copied everywhere it's used: `.fields[name].values`), a
# component its own members, a group its count field's tag and its members. FIX components
# don't refer to themselves, so this ends; $depth is a guard in case a dictionary does.
| def members($depth):
    if $depth > 20 then error("components nested more than 20 deep: a cycle?") else
    [ (.field | items[] | {kind: "field"} + ($fields[._name] | del(.values)) + {required: required}),
      (.component | items[] | {kind: "component", name: ._name, required: required}
         + {fields: ($components[._name] | members($depth + 1))}),
      (.group | items[] | {kind: "group", name: ._name, tag: $fields[._name].tag, required: required}
         + {fields: members($depth + 1)}) ]
    end;

{
  version: "FIX.\($fix._major).\($fix._minor)",
  header: ($fix.header | members(0)),
  trailer: ($fix.trailer | members(0)),
  messages: ($fix.messages.message | items
    | map({key: ._msgtype, value: {msgtype: ._msgtype, name: ._name, category: ._msgcat, fields: members(0)}})
    | from_entries),
  fields: $fields,
  tags: ($fields | map_values({key: (.tag | tostring), value: .name}) | [.[]] | from_entries)
}
