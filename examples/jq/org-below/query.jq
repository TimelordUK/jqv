# Everyone below one person, at any depth, through solid and dotted lines alike.
#
#   jq --arg root E2 -f query.jq input.json
#
# recurse follows direct_reports until it runs out. With dotted lines the org is a graph,
# not a tree, so someone can be reached twice: unique drops the repeats (and sorts by id).
# The org has no cycles (a manager is always a level up), so recurse ends.
INDEX(.id) as $by
| [$by[$root].direct_reports[] | recurse($by[.].direct_reports[])]
| unique
| map($by[.] | {id, name, title})
