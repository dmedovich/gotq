---
id: examples
title: Examples
sidebar_position: 7
---

## Common requests

Assume the endpoint exposes `id`, `name`, `age`, `active`, and `createdAt`:

```http
GET /users?filter=active eq true
GET /users?filter=age gte 18 and age lt 65
GET /users?filter=(name contains 'ann' or name contains 'ana') and active eq true
GET /users?filter=id in (1,2,3)&sort=-createdAt,name
GET /users?sort=-createdAt&limit=20&offset=40
GET /users?search=ann&count=true
```

## A complete `net/http` handler

Build the engine once and reuse it across requests:

```go
type User struct {
    ID        uint      `json:"id"`
    TenantID  uint      `json:"-"`
    Name      string    `json:"name"`
    Age       int       `json:"age"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"createdAt"`
}

func newUserEngine(db *gorm.DB) (*query.Engine[User], error) {
    return query.New(db, query.Config[User]{
        Policy: query.Schema[User]().
            Expose("id", query.Sortable()).
            Expose("name", query.Filterable(query.Eq, query.Contains), query.Sortable()).
            Expose("age", query.Filterable(query.Eq, query.Gt, query.Gte, query.Lt, query.Lte)).
            Expose("active", query.Filterable(query.Eq)).
            Expose("createdAt", query.Filterable(query.Gte, query.Lte), query.Sortable()),
        DefaultLimit: 25,
        MaxLimit:     100,
        MaxOffset:    100_000,
        AllowCount:   true,
    })
}

func listUsers(db *gorm.DB, users *query.Engine[User]) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tenantID := r.Context().Value(tenantKey{}).(uint)
        base := db.Where("tenant_id = ?", tenantID)

        page, err := users.From(base).List(r.Context(), r.URL.Query())
        if err != nil {
            queryhttp.WriteError(w, err)
            return
        }

        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        _ = json.NewEncoder(w).Encode(page)
    }
}
```

The base scope applies to data, count, search, and cursor continuation queries.

## Relationships

Expose each relationship with its own policy:

```go
companies := query.Schema[Company]().
    Expose("name", query.Filterable(query.Eq, query.Contains), query.Sortable())

orders := query.Schema[Order]().
    Expose("status", query.Filterable(query.Eq, query.In)).
    Expose("total", query.Filterable(query.Gt, query.Gte))

usersPolicy := query.Schema[User]().
    Expose("id", query.Sortable()).
    Relation("company", companies).
    Relation("orders", orders)
```

```http
GET /users?filter=company/name eq 'Acme'&sort=company/name
GET /users?filter=orders/any(o: o/total gt 100)
GET /users?filter=orders/all(o: o/status eq 'paid')
```

## Custom search

Search is endpoint-owned code. Keep client values bound:

```go
Search: func(ctx context.Context, db *gorm.DB, term string) (*gorm.DB, error) {
    pattern := "%" + term + "%"
    return db.Where("name LIKE ?", pattern), nil
},
```

The callback receives the request context and current base scope. Return an
error instead of falling back to an unscoped query.

## Parse and apply manually

Use the lower-level API when the application owns execution:

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
return scope.Find(&result).Error
```

`query.Apply` supports filter, sort, limit, and offset. Search callbacks and
cursor pagination use `Engine`.

## Framework adapters

Compile-tested handlers for `net/http`, Gin, Echo, and Fiber live in
`examples/frameworks`. Tenant-scoped and relationship examples live in
`examples/production`.
