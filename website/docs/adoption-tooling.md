---
id: adoption-tooling
title: Discovery and adoption tooling
sidebar_position: 8
---

## Describe an endpoint safely

`Engine.Describe` returns a detached, deterministic view containing public
field names, kinds, allowed operators, sortability, endpoint capabilities, and
effective limits:

```go
description := users.Describe()
```

The description intentionally has no Go field names, database columns, tables,
or SQL. Mutating it cannot change the engine policy. Explicit relationship
entries recursively contain only their nested public schema and cardinality.

## Generate OpenAPI 3.1

The dependency-free `openapi` package generates a GET operation or a complete
single-path document:

```go
document, err := openapi.Generate(
    "Users",
    "1.0.0",
    "/users",
    users.Describe(),
)
```

Canonical parameters use standard OpenAPI schemas. `x-gotq-fields`,
`x-gotq-schema`, `x-gotq-limits`, and related extensions retain operator and
byte-limit semantics that JSON Schema cannot express accurately. Enabled
compatibility aliases are emitted as deprecated parameters.

## Validate policy in application CI

Bind the complete endpoint config using the application's actual GORM setup:

```go
func TestUserQueryPolicy(t *testing.T) {
    if err := query.ValidateConfig(db, userQueryConfig); err != nil {
        t.Fatal(err)
    }
}
```

This catches renamed model fields, naming-strategy changes, invalid columns,
unsupported types, and invalid endpoint limits before deployment.

## Serialize errors

For `net/http`:

```go
page, err := users.List(r.Context(), r.URL.Query())
if err != nil {
    queryhttp.WriteError(w, err)
    return
}
```

Gin, Echo, and Fiber adapters can use `queryhttp.Response(err)` and let the
framework serialize its returned safe payload. Compile-tested handlers live in
`examples/frameworks`, a separate module so framework packages never become
core dependencies.

## Run the parser playground

```bash
go run ./cmd/gotq-playground
```

Open `http://127.0.0.1:8080`. The playground parses V1 syntax and source
positions locally. It needs neither a model nor a database, so it does not
perform endpoint-specific field or operator validation.

## Consume conformance fixtures

`conformance/v1/queries.json` contains accepted and rejected decoded HTTP
queries, alias mode, stable error codes, and parameter names. Downstream
framework integrations can execute the file directly as an adapter contract.
