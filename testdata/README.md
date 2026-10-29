# Test documents

Real-shaped JSON used by tests and for trying jqv by hand. Add files as they turn up in
daily use (DESIGN §7.1) and give each a line here: what it is, and which test leans on it.

| File | What | Used by |
|---|---|---|
| `orders.json` | Three trade orders: nested instrument, fills arrays, nulls, an error object | `cmd/jqv` tests, `examples/jq` |
| `FIX4.4.json` | The FIX 4.4 dictionary (QuickFIX's XML) run through an online XML→JSON converter: 513 KB, 19.6k rows fully open, deep and regular. Good for paging, search, and as a reference for XML mapping (C-014) | by hand |
| `nobel_prize.json` | Nobel prizes by year and category, laureates nested; 235 KB, minified | by hand |
| `USD.json` | Exchange rates: one flat object of ~160 currencies, minified | by hand |
| `users.schema.json` | A small JSON Schema (keys like `$id`, `$schema` that need quoting in jq paths) | by hand |
| `users_100.json` | 100 users with friends and hobbies, minified on one line (downloaded sample data) | `TestFoldUsersToOlivia` (V-011) |
