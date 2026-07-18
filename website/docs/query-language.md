---
id: query-language
title: Query language
sidebar_position: 3
---

| Parameter | Form | When omitted in `List` |
| --- | --- | --- |
| `filter` | one expression | no filter |
| `sort` | `-createdAt,name` | primary-key order |
| `limit` | non-negative integer | engine default |
| `offset` | non-negative integer | zero |
| `cursor` | opaque base64url token | start of effective order |
| `count` | exactly `true` or `false` | no count |
| `search` | non-empty UTF-8 text | no search |

Known parameters occur once. Unknown parameters are ignored. The opt-in
`WithCompatibilityAliases` accepts `orderby`, `top`, and `skip`. `cursor` and
`offset` cannot be combined.

## Filter

```text
expression  = or-expression
or          = and { "or" and }
and         = primary { "and" primary }
primary     = comparison | "(" expression ")"
comparison  = field operator literal
```

`and` binds more tightly than `or`. Operators and field names are
case-sensitive.

| Kind | Default operators |
| --- | --- |
| string | `eq`, `ne`, `contains`, `startswith`, `endswith`, `in`, `not in` |
| boolean | `eq`, `ne`, `in`, `not in` |
| integer, float, time, date, decimal | `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `not in` |
| UUID/custom | `eq`, `ne`, `in`, `not in` |

Examples:

```text
name eq 'O''Brien'
active eq true
age gte -18
score lt 1.5e2
createdAt gt '2026-07-16T12:30:00Z'
eventDate eq '2026-07-16'
id in (1, 2, 3)
status not in ('blocked', 'deleted')
deletedAt is null
not active eq true
name startswith 'Ann'
```

## Sorting

```text
sort=-createdAt,name
```

A leading `-` means descending. Repeated, unknown, or non-sortable fields are
errors. `List` appends missing primary-key fields as trusted stable tie-breakers.
Every effective term uses `NULLS LAST` in ascending and descending order. See
[Cursor pagination](./cursor-pagination) for forward traversal.

## Relationship paths

To-one relationships use slash paths with no surrounding whitespace:

```text
company/country/code eq 'US'
sort=company/name
```

To-many relationships require lexical quantifiers:

```text
orders/any(o: o/total gt 100)
orders/all(o: o/status eq 'paid')
orders/any(o: o/items/any(i: i/price gte 10))
```

Predicate paths must start with the declared variable. `all` is true over an
empty collection, while a `NULL` predicate result does not satisfy `all`.
To-many sorting is not valid.

For normative EBNF and diagnostic positions, see
[`GRAMMAR.md`](https://github.com/dmedovich/gotq/blob/main/GRAMMAR.md).
