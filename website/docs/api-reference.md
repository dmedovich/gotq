---
id: api-reference
title: API reference
sidebar_position: 8
---

The symbol-level reference is
[`pkg.go.dev/github.com/dmedovich/gotq`](https://pkg.go.dev/github.com/dmedovich/gotq).
The complete exported surface is frozen and checked module-wide against the
previous release; see [Stability and support](stability-policy.md).

## Engine flow

```go
func New[T any](db *gorm.DB, config Config[T]) (*Engine[T], error)
func (e *Engine[T]) Parse(values url.Values) (Query, error)
func (e *Engine[T]) Apply(ctx context.Context, base *gorm.DB, q Query) (*gorm.DB, error)
func (e *Engine[T]) List(ctx context.Context, values url.Values) (Page[T], error)
func (e *Engine[T]) From(base *gorm.DB) Session[T]
func (e *Engine[T]) Describe() EndpointDescription
func ValidateConfig[T any](db *gorm.DB, config Config[T]) error
```

Session provides `Apply` and `List` using its base scope.

## Policy

```go
func Schema[T any]() *SchemaBuilder[T]
func (b *SchemaBuilder[T]) Expose(name string, options ...FieldOption) *SchemaBuilder[T]
func (b *SchemaBuilder[T]) Relation(name string, policy NestedPolicy, options ...RelationshipOption) *SchemaBuilder[T]
func Filterable(operators ...ComparisonOperator) FieldOption
func Sortable() FieldOption
func GoField(name string) FieldOption
func Column(name string) FieldOption
func WithCodec(codec ScalarCodec) FieldOption
func RelationGoField(name string) RelationshipOption
```

## Low-level flow

```go
func ParseHTTP(values url.Values, options ...ParseOption) (Query, error)
func Apply[T any](db *gorm.DB, schema *ModelSchema[T], q Query) (*gorm.DB, error)
func WithLimits(limits Limits) ParseOption
func WithCompatibilityAliases() ParseOption
```

`Apply` does not execute a query. Public AST nodes remain inspectable but their
interface is sealed. Manually assembled queries are validated before compilation.
Package-level `Apply` rejects cursors because decoding requires the engine's
effective primary-key tie-breakers. `PageInfo.NextCursor` is present only when
`HasMore` is true.

## Optional packages

```go
func openapi.Generate(title, version, path string, description query.EndpointDescription) (openapi.Document, error)
func openapi.NewOperation(description query.EndpointDescription) (openapi.Operation, error)
func queryhttp.Status(err error) int
func queryhttp.Response(err error) (int, any)
func queryhttp.WriteError(w http.ResponseWriter, err error)
```

`openapi` has no OpenAPI-library dependency. `queryhttp` uses only `net/http`;
framework adapters remain outside the core module.
