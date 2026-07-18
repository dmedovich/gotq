# Application examples

This module contains two complete HTTP examples. It is separate from the core
module so application dependencies do not affect gotq users.

- `tenantapp` derives every query from an authenticated tenant scope, supports
  exact count and cursor traversal, and verifies that a cursor cannot grant
  access across tenants.
- `catalogapp` exercises belongs-to, has-many, and many-to-many policy paths,
  nested quantifiers, to-one sorting, exact count, and cursor traversal.

Run them with:

```bash
go test -race ./...
```

Both examples are covered by CI.
