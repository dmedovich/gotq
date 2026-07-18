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

`List` fetches one extra row to calculate `has_more`. Count is disabled unless
`AllowCount` is true and runs only for `count=true`. It includes base scope,
filter, and search while excluding query sort, limit, and offset.
It also excludes cursor position, so every continuation can report the same
complete filtered total.
Relationship filters use correlated subqueries, so matching child rows do not
multiply the root count.

`limit=0` executes no data query but may still execute count. Most endpoints
should rely on `has_more` and enable exact count only when clients need it.
When `has_more` is true, `nextCursor` continues after the last returned item.

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
