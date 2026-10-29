# Total filled quantity per order, and across all orders: walk into nested arrays,
# collect the numbers, and add them up.
#
#   jq -c -f query.jq input.json
[.orders[] | {id, filled: ([.fills[].qty] | add // 0)}]
| {orders: ., total: (map(.filled) | add)}
