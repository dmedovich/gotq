---
id: getting-started
title: Getting started
sidebar_position: 2
---

## Install

```bash
go get github.com/dmedovich/gotq
```

gotq requires Go 1.23 or newer.

## Define endpoint policy

Build an engine once during application startup:

```go
type User struct {
    ID        uint      `json:"id"`
    Name      string    `json:"name"`
    Age       int       `json:"age"`
    CreatedAt time.Time `json:"createdAt"`
}

policy := query.Schema[User]().
    Expose("id", query.Sortable()).
    Expose("name", query.Filterable(), query.Sortable()).
    Expose("age", query.Filterable()).
    Expose("createdAt", query.Filterable(), query.Sortable())

users, err := query.New(db, query.Config[User]{
    Policy:       policy,
    DefaultLimit: 25,
    MaxLimit:     100,
    MaxOffset:    100_000,
    AllowCount:   true,
})
if err != nil {
    panic(err)
}
```

`Expose` is the client whitelist. Go types, columns, and primary keys are
inferred through the actual GORM configuration; undisclosed fields stay private.

## Execute a list

```go
func listUsers(users *query.Engine[User], db *gorm.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        base := db.Where("tenant_id = ?", tenantID(r))
        page, err := users.From(base).List(r.Context(), r.URL.Query())
        if err != nil {
            queryhttp.WriteError(w, err)
            return
        }
        _ = json.NewEncoder(w).Encode(page)
    }
}
```

The base scope is preserved for data and count. An omitted limit uses 25;
sorting is made deterministic with the model primary key.
If another page exists, return `page.nextCursor` to the client and accept it
unchanged as the next request's `cursor` while omitting `offset`.

## Try a request

```http
GET /users?filter=age gt 18 and name contains 'ann'&sort=-createdAt&limit=20&count=true
```

Advanced callers can use `Engine.Parse` and `Engine.Apply`, or the package-level
`ParseHTTP` and `Apply`, without executing a database statement.

For explicit to-one paths and to-many `any`/`all`, continue with
[Relationships](./relationships).
For deep forward traversal, continue with
[Cursor pagination](./cursor-pagination).
