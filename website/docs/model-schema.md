---
id: model-schema
title: Model policy and schema
sidebar_position: 4
---

Policy explicitly declares the case-sensitive HTTP fields and operations an
endpoint exposes. Engine construction binds it once to immutable GORM metadata.

```go
policy := query.Schema[User]().
    Expose("displayName",
        query.GoField("Name"),
        query.Filterable(query.Eq, query.Contains),
        query.Sortable(),
    )
```

When `GoField` is omitted, gotq resolves an exact JSON tag or exported Go field.
The active GORM schema supplies the database column, primary key, and naming
strategy. `Column` is an explicit validated override.

Supported inferred kinds are strings, booleans, signed/unsigned integers,
floats, `time.Time`, `DateValue`, `UUIDValue`, `DecimalValue`, named forms, and
pointers to them. `uintptr`, slices, maps, and relations are rejected unless a
custom codec explicitly validates the scalar target.

```go
type CodeCodec struct{}

func (CodeCodec) Name() string { return "code" }
func (CodeCodec) ValidateType(target reflect.Type) error { /* ... */ }
func (CodeCodec) ParseLiteral(l query.Literal, target reflect.Type) (any, error) {
    // Convert syntax only; no SQL or identifier access exists here.
}

policy.Expose("code", query.WithCodec(CodeCodec{}), query.Filterable())
```

For unusual low-level mappings, `Field(name, kind, options...)` retains an
explicit kind. `Build` supports that pre-release explicit schema workflow;
`Bind(db)` and `New` use actual GORM metadata and support `Expose`.

Configuration failures use `invalid_schema` and occur before an engine is
returned. A field present in the model but absent from policy is inaccessible.

## Relationship policy

`Relation("company", companyPolicy)` explicitly exposes one GORM association
and its nested whitelist. `RelationGoField` maps a public name to a differently
named Go association. Relationship binding always requires the active GORM
database; `Build` alone cannot validate tables, keys, join tables, cardinality,
or soft-delete metadata.

Association fields that are not declared with `Relation` remain private even
when GORM discovers them. Polymorphic and cyclic policy graphs are rejected.
