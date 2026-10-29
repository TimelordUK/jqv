# Test documents

Real-shaped JSON used by tests and for trying jqv by hand. Add files as they turn up in
daily use (DESIGN §7.1) and give each a line here: what it is, and which test leans on it.

| File | What | Used by |
|---|---|---|
| `orders.json` | Three trade orders: nested instrument, fills arrays, nulls, an error object | `cmd/jqv` tests, `examples/jq` |
| `users_100.json` | 100 users with friends and hobbies, minified on one line (downloaded sample data) | `TestFoldUsersToOlivia` (V-011) |
