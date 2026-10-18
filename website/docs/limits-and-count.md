---
id: limits-and-count
title: Limits and count
sidebar_position: 6
---

Low-level defaults are:

```go
query.Limits{
    MaxQueryBytes:      16 << 10,
    MaxFilterBytes:     8 << 10,
    MaxTokens:          512,
    MaxLiteralBytes:    4 << 10,
    MaxInValues:        100,
    MaxLimit:           100,
    MaxOffset:          100_000,
    MaxSortTerms:       5,
    MaxSearchBytes:     256,
    MaxExpressionDepth: 16,
    MaxNodes:           100,
    MaxPathDepth:       8,
    MaxQuantifierDepth: 4,
    MaxCursorBytes:     4 << 10,
}
```

All custom values passed to `WithLimits` are positive. `Engine` additionally
requires a positive `DefaultLimit`, `MaxLimit`, and `MaxOffset`; an omitted list
limit can therefore never produce an unbounded query.

Decoded query bytes include parameter names and values, even for unknown
parameters. Filter bytes are rejected before lexing; token and literal bounds
then constrain parser allocations and work.
Each `in` or `not in` list is independently bounded by `MaxInValues`.
Cursor bytes are rejected before base64url/JSON decoding.

`Engine.Apply` and `Session.Apply` enforce the receiving endpoint's limits,
including for a manually constructed `Query` or a query parsed by another
engine. If the original parser had stricter limits, those remain in force.
Original decoded HTTP size (including unknown parameters), filter bytes, and
tokens are checked when transferring a parsed query to a stricter endpoint.
For a hand-built AST there is no original HTTP representation: AST depth,
nodes, paths, sets, literal values, and other structured limits apply instead.
Package-level `Apply` continues to use parser limits or the low-level defaults.

## Default sort

Set `Config.DefaultSort` to the same syntax as the HTTP `sort` parameter, for
example `"-createdAt,name"`. Every field must be explicitly sortable. `New` and
`ValidateConfig` reject unknown fields, to-many sort paths, duplicate terms,
and sort/path limit violations during startup with `invalid_schema`.

The default applies to `List` and engine/session `Apply` when no client sort
was provided. An explicit client sort (including enabled `orderby` aliases)
replaces the default completely. An empty `DefaultSort` preserves the existing
primary-key order. `Describe().DefaultSort` and the OpenAPI `sort` schema expose
the configured default using only public names.

`List` fetches one extra row to calculate `hasMore`. Count is disabled unless
`AllowCount` is true and runs only for `count=true`. It includes base scope,
filter, and search while excluding query sort, limit, and offset.
It also excludes cursor position, so every continuation can report the same
complete filtered total.
Relationship filters use correlated subqueries, so matching child rows do not
multiply the root count.

`limit=0` executes no data query but may still execute count. Most endpoints
should rely on `hasMore` and enable exact count only when clients need it.
When `hasMore` is true, `nextCursor` continues after the last returned item.

## Stable benchmark baseline

On the published local reference host (AMD Ryzen 5 2600, Linux amd64,
Go 1.26.5), representative v1.0 results were:

| Operation | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| parse HTTP query | 2,567 | 1,744 | 13 |
| parse maximum valid filter | 53,611 | 76,608 | 158 |
| parse + apply engine plan | 9,215 | 6,368 | 64 |
| relationship parse + apply | 15,533 | 9,135 | 83 |
| cursor parse + apply | 21,693 | 14,106 | 102 |
| SQLite list | 345,911 | 31,489 | 699 |

Benchmarks are regression checks, not latency promises. The full command,
environment, and additional cases are in the
[`benchmark record`](https://github.com/dmedovich/gotq/blob/main/docs/benchmarks.md).
