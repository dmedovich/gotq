---
id: relationships
title: Relationships
sidebar_position: 5
---

Relationships are opt-in at every path segment. GORM metadata resolves trusted
tables and keys only after an endpoint supplies a nested whitelist.

## Declare nested policy

```go
companyPolicy := query.Schema[Company]().
    Expose("name", query.Filterable(query.Eq), query.Sortable())

itemPolicy := query.Schema[Item]().
    Expose("price", query.Filterable(query.Gte))

orderPolicy := query.Schema[Order]().
    Expose("total", query.Filterable(query.Gt, query.Gte)).
    Expose("status", query.Filterable(query.Eq)).
    Relation("items", itemPolicy)

userPolicy := query.Schema[User]().
    Expose("id", query.Sortable()).
    Relation("company", companyPolicy).
    Relation("orders", orderPolicy)
```

Public relationship names resolve by JSON tag or exported Go association.
`RelationGoField("Employer")` provides an explicit override. Missing,
mismatched, polymorphic, cyclic, and incomplete association metadata fails
engine construction.

## To-one paths

```http
GET /users?filter=company/name eq 'Acme'&sort=company/name
```

Every relationship and the final field must be exposed by its policy. To-one
filters use correlated existence checks. To-one sorting uses trusted,
deduplicated association joins and retains root primary-key tie-breakers.

## To-many quantifiers

```http
GET /users?filter=orders/any(o: o/total gt 100)
GET /users?filter=orders/all(o: o/status eq 'paid')
GET /users?filter=orders/any(o: o/items/any(i: i/price gte 10))
```

The variable after `(` creates a lexical scope. Predicate paths must start with
that variable, and nested variables cannot shadow active variables.

- `any` is true when at least one related row satisfies the predicate.
- `all` is true when no related row violates it, including an empty collection.
- A SQL `NULL` predicate result does not satisfy `all`.

To-many filters compile as correlated subqueries, not root joins, so multiple
matching children cannot duplicate root items or inflate count. Sorting through
a to-many relationship is rejected.

## Association semantics

Has-one, belongs-to, has-many, many-to-many, nested, nullable foreign-key, and
composite-key relationships use validated GORM metadata. Deterministic internal
aliases cannot collide with discovered table/join-table names.

Standard `gorm.DeletedAt` rows are excluded from related subqueries by default.
An application-owned `Unscoped` base consistently includes root and related
soft-deleted rows. Tenant isolation must be expressed by the caller base scope
and, when keys are tenant-relative, by composite GORM relationship references.

Polymorphic associations and aggregate to-many sorting are rejected during
binding.
