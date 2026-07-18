---
id: intro
title: gotq
slug: /
sidebar_position: 1
---

gotq is a safe query engine for GORM list endpoints. It parses untrusted HTTP
query parameters, binds them to an explicit endpoint policy and trusted GORM
metadata, and either returns a derived scope or executes a bounded page.

```text
url.Values → parse → bind → validated plan → Apply → optional List
```

The endpoint policy is the trust boundary. A model field or relationship is not
client-accessible until the endpoint explicitly exposes it.

## Features

- `filter` with comparisons, `and`, `or`, and parentheses;
- `not`, `in`/`not in`, null predicates, and prefix/suffix matching;
- exact date, UUID, decimal, and constrained custom scalar conversion;
- `sort=-createdAt,name` with stable primary-key tie-breaking;
- bounded `limit` and `offset` with a safe engine default;
- opaque forward cursors with portable `NULLS LAST` keyset ordering;
- optional `count` and endpoint-defined `search`;
- `Engine[T]`, `From(base)`, `List`, and a typed `Page[T]`;
- low-level `ParseHTTP` and `Apply` without database execution;
- structured errors, bound values, inferred GORM columns, and explicit policy.

Explicit relationship paths and collection quantifiers are available.
Cursor signing/encryption, backward traversal, raw SQL, JSONB, and arbitrary
client operators remain out of scope.

[Build your first engine →](/docs/getting-started)
