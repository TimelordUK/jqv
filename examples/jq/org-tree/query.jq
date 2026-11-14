# A flat org chart (employees linked by id) as a nested tree from one person down.
# Children follow the solid line only (the first id in reports_to), so everyone appears
# once; dotted-line reports are listed by id under `dotted` instead of nested twice.
#
#   jq --arg root E1 -f query.jq input.json
#
# INDEX(.id) makes a lookup object {id: employee}, built once; the recursive def then
# walks direct_reports through it. In jqv, without --arg: "E1" as $root | ...
INDEX(.id) as $by
| def tree:
    .id as $me
    | {id, name, title,
       reports: [.direct_reports[] | $by[.] | select(.reports_to[0] == $me) | tree],
       dotted: [.direct_reports[] | select($by[.].reports_to[0] != $me)]};
  $by[$root] | tree
