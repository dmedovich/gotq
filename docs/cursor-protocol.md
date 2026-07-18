# Cursor protocol v1

Status: stable v1.0 protocol
Date: 2026-07-16

This document is the normative compatibility contract for forward cursor
pagination. Cursor payload version `1` is independent of the query language's
`SyntaxVersion == "v1"`.

## Transport encoding

A cursor is canonical JSON encoded with Go's `encoding/json`, then encoded with
unpadded RFC 4648 base64url (`base64.RawURLEncoding`). It contains only ASCII
letters, digits, `-`, and `_`. Alternate JSON whitespace, field order, number
forms, unknown fields, trailing values, base64 alphabet, and `=` padding are
rejected.

The decoded object has exactly this field order and shape:

```json
{"v":1,"s":"<sort-signature>","k":[<key>,...]}
```

- `v` is the integer payload version and must equal `1`.
- `s` is a base64url-encoded 144-bit SHA-256 prefix over the endpoint's trusted
  model metadata and complete effective sort contract.
- `k` contains one typed JSON value per effective sort key, including hidden
  primary-key tie-breakers. SQL `NULL` is JSON `null`.

The signature is deliberately opaque. The payload contains no public field
path, Go field, database column, table, relationship, join, or alias name.
Changing a requested sort path or direction, trusted storage mapping, scalar
Go type, hidden tie-breaker, or cursor protocol version changes the signature.
The key values themselves are not confidential and can include otherwise
unexposed primary-key values used as tie-breakers. Applications for which those
values are sensitive encrypt or otherwise wrap the complete cursor.

Each non-null key must unmarshal into its bound Go scalar type and marshal back
to the identical JSON bytes. This covers strings, booleans, signed and unsigned
integers, finite floats, time, date, UUID, decimal, and JSON-round-trippable
custom scalar types without decoding through an untyped `float64`.

## Ordering and continuation

The effective order is the requested sort followed by every missing root
primary-key column in ascending order. All keys use `NULLS LAST`, independently
of ascending or descending direction. Gotq emits portable `CASE WHEN key IS
NULL` ordering and a lexicographic keyset predicate equivalent to:

```text
(k1 after c1)
OR (k1 equal c1 AND k2 after c2)
OR ...
```

For a non-null cursor key, `after` includes later non-null values in the term's
direction plus SQL `NULL`. For a null cursor key there is no later value on that
term, but equality permits subsequent tie-breakers to advance. Equality uses
`IS NULL` for a null cursor key.

`PageInfo.NextCursor` is non-empty only when `HasMore` is true. It represents
the last returned item, never the extra item fetched to detect another page.
Clients continue by sending the same `sort` and the returned `cursor`, while
omitting `offset`. `limit` may change between requests. Filters, search terms,
and caller-owned authorization scopes are evaluated again and are not embedded
in the cursor signature.

## Security and compatibility

Cursors are position data, not authority. They are bounded by
`MaxCursorBytes`, parsed canonically, type-checked, sort-bound, and used only as
bound SQL values. Every request still derives from the caller's current base
scope. Invalid cursors fail before database execution with `invalid_cursor`;
an encoded value above the configured bound fails with `limit_exceeded`.

Core cursors are not signed, encrypted, expiring, or bound to an application
filter/user identity. Applications needing those properties wrap the complete
opaque gotq cursor outside the core module. Cursor payload v1 will not change
in the stable v1 line. A future incompatible payload uses a different `v`; v1
decoders reject it rather than guessing.
