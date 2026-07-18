---
id: errors-and-security
title: Errors and security
sidebar_position: 5
---

## Structured errors

Client query, schema, and engine execution failures use `*query.Error`.

```go
var queryErr *query.Error
if errors.As(err, &queryErr) {
    response := struct {
        Code     query.ErrorCode  `json:"code"`
        Message  string           `json:"message"`
        Position *query.Position  `json:"position,omitempty"`
    }{
        Code: queryErr.Code,
        Message: queryErr.Message,
        Position: queryErr.Position,
    }
    _ = json.NewEncoder(w).Encode(response)
}
```

The error type itself has stable JSON field names. Optional kind and operator
metadata use pointers and serialize as strings.

```json
{
  "code": "operator_not_allowed",
  "parameter": "filter",
  "position": {"offset": 4, "line": 1, "column": 5},
  "message": "operator \"contains\" cannot be used with field \"age\" of type int\nallowed operators: eq, ne, gt, gte, lt, lte, in, not in",
  "field": "age",
  "kind": "int",
  "operator": "contains",
  "allowedOperators": ["eq", "ne", "gt", "gte", "lt", "lte", "in", "not in"]
}
```

Machine logic should switch on `Code`, not `Message`.
The code registry is frozen for v1; new codes require a minor release and a
documented fallback, while removal or semantic reuse requires a major release.

| Code | Meaning |
| --- | --- |
| `invalid_parameter` | malformed or duplicate HTTP parameter |
| `invalid_token` | filter cannot be tokenized |
| `invalid_syntax` | token sequence is not a complete expression |
| `limit_exceeded` | pagination or expression complexity is too large |
| `unknown_field` | public name is absent from the schema |
| `field_not_filterable` | filtering capability is disabled |
| `field_not_sortable` | ordering capability is disabled |
| `operator_not_allowed` | operator is outside the field whitelist |
| `invalid_literal` | literal cannot convert to the exact Go field type |
| `invalid_relationship` | relationship is unknown, unexposed, incorrectly scoped, or used with an invalid cardinality operation |
| `invalid_cursor` | cursor encoding, version, sort signature, key count, or typed value is invalid |
| `invalid_schema` | developer configuration is invalid |
| `execution_failed` | database or trusted search execution failed |

## Security invariants

- Client field text is resolved through the model schema and never copied into
  SQL.
- Public relationship text never becomes a table, join, key, or alias; those
  identifiers come only from validated GORM metadata.
- Database columns are simple identifiers from an immutable whitelist and are
  quoted through GORM clause types.
- Literal values always use GORM bound-variable slots.
- Operators and sort directions are closed enums, not strings passed to SQL.
- `contains` escapes `%`, `_`, and `!` and emits `ESCAPE '!'`, so client text is
  a literal substring rather than a wildcard pattern.
- Syntax, type, capability, and complexity validation completes before scopes
  are compiled.
- Low-level parsing and applying scopes do not execute database I/O.
- `List` preserves caller base scopes for data, count, and search.
- Cursor values are size-bounded, canonical, typed, sort-bound, and remain
  bound SQL variables inside the current base scope.
- Execution causes support `Unwrap` but are never serialized.

Case sensitivity of `contains` follows the database and column collation in v1.
