# Error compatibility policy

Status: stable v1.0 contract
Date: 2026-07-16

Client and engine failures use `*query.Error`. Machine logic must switch on
`Code`; `Message` is diagnostic prose and may improve without a compatibility
promise. Optional typed fields provide context but do not replace the code.

## V1 code registry

| Code | Category | Safe HTTP mapping |
| --- | --- | ---: |
| `invalid_parameter` | malformed, duplicate, conflicting, or unsupported parameter value | 400 |
| `invalid_token` | filter lexical failure | 400 |
| `invalid_syntax` | incomplete or invalid grammar | 400 |
| `limit_exceeded` | configured request/pagination/AST budget exceeded | 400 |
| `unknown_field` | public scalar name is absent | 400 |
| `field_not_filterable` | declared field lacks filter capability | 400 |
| `field_not_sortable` | declared field lacks sort capability | 400 |
| `operator_not_allowed` | operator is outside the field policy | 400 |
| `invalid_literal` | literal cannot convert to the exact declared scalar | 400 |
| `invalid_relationship` | path, cardinality, variable scope, or relation policy is invalid | 400 |
| `invalid_cursor` | cursor syntax, version, signature, arity, type, or canonical form is invalid | 400 |
| `invalid_schema` | application policy/configuration or GORM metadata is invalid | 500 |
| `execution_failed` | database or trusted callback execution failed | 500 |

`queryhttp` also uses the package-local `internal_error` payload for unknown or
unsafe failures; it is not a `query.ErrorCode`. It deliberately hides wrapped
causes and maps schema/execution failures to HTTP 500.

## Compatibility rules

- An existing code never changes category or broad meaning in v1.
- A patch release may make a more specific existing code apply to an edge case
  only when that corrects a defect without exposing sensitive information.
- A new code requires a minor release, a fallback rule, conformance vectors,
  HTTP mapping, and upgrade notes.
- Removal, rename, or merging codes requires a major release.
- `Position.Offset` is a zero-based byte offset in the decoded parameter;
  `Line` and `Column` are one-based, with column measured in bytes.
- `Cause` participates in `errors.Is`/`errors.As` through `Unwrap` but always has
  JSON tag `-` and must not cross an HTTP boundary.

The exact registry and endpoint-description JSON shape have executable freeze
tests in `contract_freeze_test.go`.
