---
id: stability-policy
title: Stability and support
sidebar_position: 10
---

The v1 release freezes the exported `query`, `openapi`, and `queryhttp` APIs,
the V1 grammar, error-code registry, schema-description JSON, and cursor payload
v1.

Patch releases preserve source and behavioral compatibility. Minor releases
may add reviewed backward-compatible API. Removal, rename, signature changes,
or reinterpretation require a new major version except when an urgent security
fix cannot be safely delivered another way.

Every release runs a module-wide `apidiff` against the previous tag. Public API
changes require explicit review.

## Deprecation

A deprecation includes a replacement and upgrade example. Except for urgent
security removal, it remains functional for at least two minor releases and six
months, whichever is longer, and is removed only in a major release.

`orderby`, `top`, and `skip` entered v1 already deprecated. They remain explicit
opt-ins throughout v1; new clients use `sort`, `limit`, and `offset`.

## Wire compatibility

Consumers should ignore unknown JSON object fields. Gotq will not remove,
rename, change the JSON type of, or reinterpret an existing error, page, schema
description, OpenAPI extension, or cursor v1 field during the v1 line. A new
incompatible cursor encoding gets a new version and old decoders reject it.

The complete normative policies are
[`docs/api-policy.md`](https://github.com/dmedovich/gotq/blob/main/docs/api-policy.md),
[`docs/error-policy.md`](https://github.com/dmedovich/gotq/blob/main/docs/error-policy.md),
and [`SECURITY.md`](https://github.com/dmedovich/gotq/blob/main/SECURITY.md).
