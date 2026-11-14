# The management chain from one person up to the top, one array per route: someone with
# a dotted line has two managers, so two chains.
#
#   jq --arg root E5 -f query.jq input.json
#
# chains is recursive and a generator: $by[.reports_to[]] yields each manager in turn,
# and each one's chains come back with this person put in front.
INDEX(.id) as $by
| def chains:
    if .reports_to == [] then [.id]
    else .id as $me | $by[.reports_to[]] | chains | [$me] + .
    end;
  $by[$root] | chains
