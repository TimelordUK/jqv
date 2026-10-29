# Every message belonging to one parent order, from a feed where a child names its
# parent but never the root (FlexTrade drop copy: gRPC messages logged one per line).
#
# One streaming pass: keep a set of ids known to be in the tree, starting from $root.
# A message joins when its own id or its parent's id is already in the set. Only the set
# is held in memory, so this works on a multi-GB log. It assumes parents appear before
# their children, which a log written in time order guarantees.
#
#   jq -c -n --arg root P1 -f query.jq input.jsonl
foreach inputs as $m ({ids: {($root): true}};
  if .ids[$m.orderId] or .ids[$m.parentId // ""]
  then .ids[$m.orderId] = true | .out = $m
  else .out = null
  end;
  .out // empty)
