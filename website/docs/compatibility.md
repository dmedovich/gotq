---
id: compatibility
title: Compatibility
sidebar_position: 9
---

gotq requires Go 1.25 or newer. CI targets Go 1.25.14 for source compatibility
and Go 1.26.9 / Go 1.27.2 for supported production toolchains. Go 1.25 is an
end-of-life compatibility lane, not a security-supported production toolchain.
Production builds should use a supported Go release with all security patches;
the release vulnerability scan uses Go 1.27.2. See the
[Go release history](https://go.dev/doc/devel/release).
These are the current branch's validation targets. Support for a release
requires the complete race, dialect, API, and release campaign to pass.

Every test run checks dialect quoting and bound variables in DryRun mode. CI and
release workflows additionally execute real PostgreSQL 17 and MySQL 8.4
cursor/relationship conformance tests; SQLite integration runs in-process.

The supported database range is embedded SQLite 3.45.1 with sqlite driver
v1.6.0, PostgreSQL 17.x, and MySQL 8.4.x LTS. The module pins GORM v1.31.2,
PostgreSQL driver v1.6.3, and SQLite/MySQL drivers v1.6.0.

Cursor payload v1 and `NULLS LAST` traversal are protocol contracts across all
three dialects.

Stable v1 freezes exported Go APIs, query grammar, parameter semantics, error
codes, cursor payload v1, and serialized page/description fields. Stable
changes follow Semantic Versioning and the published deprecation policy.
