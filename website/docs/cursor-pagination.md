---
id: cursor-pagination
title: Cursor pagination
sidebar_position: 7
---

Gotq returns an opaque forward cursor whenever a page has more rows:

```json
{
  "items": [{"id": 42, "createdAt": "2026-07-16T12:00:00Z"}],
  "page": {
    "limit": 1,
    "offset": 0,
    "hasMore": true,
    "nextCursor": "eyJ2IjoxLCJzIjoiLi4u"
  }
}
```

Send it unchanged with the same sort and omit `offset`:

```http
GET /users?sort=-createdAt&limit=20&cursor=eyJ2IjoxLCJzIjoiLi4u
```

The first request omits both cursor and offset. `limit` may change between
pages. Keep filter and search stable unless you deliberately want to continue
the same sort position over a different result set. Exact count describes the
complete filtered result before cursor/page constraints.

## Stable order

Gotq appends every missing root primary-key column as an ascending tie-breaker.
All requested and hidden keys use `NULLS LAST` in either direction, producing
the same traversal on SQLite, PostgreSQL, and MySQL. Duplicate values,
relationships, nullable fields, and composite primary keys therefore advance
without relying on offset.

The effective endpoint sort replaces any `Order` already present on the caller
base scope. Tenant/authorization filters, `Select`, `Omit`, `Distinct`, search,
and other non-order scopes remain in force.

## Validation and security

The cursor is canonical unpadded base64url JSON v1 containing an opaque sort
signature and typed key values. It exposes no field, Go, column, table, join, or
alias names. Malformed, tampered, wrong-version, wrong-sort, or wrong-type
cursors fail with `invalid_cursor` before database execution. Encoded size is
bounded by `MaxCursorBytes` (4096 by default).

A cursor is position data, not authority. It is unsigned, unencrypted,
non-expiring, and not bound to a user/filter. Every continuation still applies
the current caller base scope. Wrap the complete opaque token in your transport
layer if the application needs integrity, confidentiality, expiry, or identity
binding.
Decoded keys may include an otherwise unexposed primary-key tie-breaker;
base64url does not make those values confidential.
Stored float sort keys must be finite; non-finite values have no canonical JSON
or portable cross-dialect order.

The byte-level compatibility contract is published in
[`docs/cursor-protocol.md`](https://github.com/dmedovich/gotq/blob/main/docs/cursor-protocol.md).
