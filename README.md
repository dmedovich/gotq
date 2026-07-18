<h1 align="center">gotq</h1>

<p align="center">
  <em>Safe, schema-first HTTP queries for GORM.</em>
</p>

<p align="center">
  <a href="https://github.com/dmedovich/gotq/actions/workflows/ci.yml"><img src="https://github.com/dmedovich/gotq/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/dmedovich/gotq"><img src="https://pkg.go.dev/badge/github.com/dmedovich/gotq.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/dmedovich/gotq"><img src="https://goreportcard.com/badge/github.com/dmedovich/gotq" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/dmedovich/gotq" alt="License"></a>
</p>

gotq turns `filter`, `sort`, `limit`, `offset`, `cursor`, `count`, and `search`
query parameters into validated GORM queries. Clients can only use fields,
relationships, and operators explicitly enabled by the endpoint.

## Install

```bash
go get github.com/dmedovich/gotq
```

gotq requires Go 1.23 or newer.

## Quick start

Define the endpoint policy once at startup:

```go
type User struct {
	ID        uint      `json:"id"`
	TenantID  uint      `json:"-"`
	Name      string    `json:"name"`
	Age       int       `json:"age"`
	CreatedAt time.Time `json:"createdAt"`
}

users, err := query.New(db, query.Config[User]{
	Policy: query.Schema[User]().
		Expose("id", query.Sortable()).
		Expose("name", query.Filterable(query.Eq, query.Contains), query.Sortable()).
		Expose("age", query.Filterable(query.Eq, query.Gt, query.Gte, query.Lt, query.Lte)).
		Expose("createdAt", query.Filterable(), query.Sortable()),
	DefaultLimit: 25,
	MaxLimit:     100,
	MaxOffset:    100_000,
	AllowCount:   true,
})
if err != nil {
	panic(err)
}
```

Use it in a handler. A caller-owned base scope is preserved for both data and
count queries:

```go
func listUsers(w http.ResponseWriter, r *http.Request) {
	base := db.Where("tenant_id = ?", tenantID(r))
	page, err := users.From(base).List(r.Context(), r.URL.Query())
	if err != nil {
		queryhttp.WriteError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
```

Example requests:

```http
GET /users?filter=age gte 18 and name contains 'ann'&sort=-createdAt&limit=20
GET /users?filter=id in (1,2,3)&count=true
GET /users?sort=-createdAt&limit=20&cursor=eyJ2IjoxLCJzIjoiLi4u
```

## Relationships

Relationships are private until they receive a nested policy:

```go
companies := query.Schema[Company]().
	Expose("name", query.Filterable(query.Eq), query.Sortable())

orders := query.Schema[Order]().
	Expose("total", query.Filterable(query.Gt, query.Gte))

usersPolicy := query.Schema[User]().
	Expose("id", query.Sortable()).
	Relation("company", companies).
	Relation("orders", orders)
```

The policy enables requests such as:

```http
GET /users?filter=company/name eq 'Acme'&sort=company/name
GET /users?filter=orders/any(o: o/total gte 100)
```

## Parse and apply without executing

Applications that manage execution themselves can use the lower-level API:

```go
schema, err := usersPolicy.Bind(db)
if err != nil {
	return err
}

parsed, err := query.ParseHTTP(r.URL.Query())
if err != nil {
	return err
}

scope, err := query.Apply(db, schema, parsed)
if err != nil {
	return err
}
err = scope.Find(&result).Error
```

## What it protects

- Values are passed to GORM as bound parameters.
- Public names resolve through an explicit policy and GORM metadata.
- Operators and literals are checked against the model field type.
- Input size, expression complexity, page size, and offsets are bounded.
- Sorting is deterministic, with primary-key tie-breakers.
- Forward cursors support deep pagination without large offsets.
- Errors have stable codes and source positions suitable for HTTP responses.

See the [documentation](https://dmedovich.github.io/gotq/) for the complete
[query language](https://dmedovich.github.io/gotq/docs/query-language),
[schema rules](https://dmedovich.github.io/gotq/docs/model-schema), and
[examples](https://dmedovich.github.io/gotq/docs/examples).

Run the database-free parser playground:

```bash
go run ./cmd/gotq-playground
```
