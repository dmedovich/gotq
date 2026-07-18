---
id: compatibility
title: Compatibility
sidebar_position: 9
---

gotq retains source compatibility with Go 1.23 and is tested on Go 1.23,
Go 1.25.12, and Go 1.26.5. Go 1.23 is an end-of-life compatibility lane, not a
security-supported production toolchain. Production builds for this release
must use fully patched Go 1.25.12 or Go 1.26.5.
The declared source-compatibility range ends at Go 1.26.5 until a newer
toolchain completes the full CI campaign.
Stable v1.0 pins and tests GORM v1.31.2 and SQLite, PostgreSQL, and MySQL drivers
v1.6.0. Newer dependency versions enter the supported range only after the
complete race, dialect, API, and release campaign passes.

Every test run checks dialect quoting and bound variables in DryRun mode. CI and
release workflows additionally execute real PostgreSQL 17 and MySQL 8.4
cursor/relationship conformance tests; SQLite integration runs in-process.

The supported database range is embedded SQLite 3.45.1 with sqlite driver
v1.6.0, PostgreSQL 17.x, and MySQL 8.4.x LTS. The module pins GORM v1.31.2 and
all three GORM drivers at v1.6.0.

Cursor payload v1 and `NULLS LAST` traversal are protocol contracts across all
three dialects.

Stable v1 freezes exported Go APIs, query grammar, parameter semantics, error
codes, cursor payload v1, and serialized page/description fields. Stable
changes follow Semantic Versioning and the published deprecation policy.
