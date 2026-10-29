# Pick a few fields from each order, flattening the nested instrument, and keep only the
# orders you care about. The everyday "give me a grid of these columns" query.
#
#   jq -c -f query.jq input.json
.orders[]
| select(.status != "rejected")
| {id, symbol: .instrument.symbol, side, qty, price}
