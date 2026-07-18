# Compatibility policy

Status: stable v1.0 support contract
Date: 2026-07-16

## Go and GORM

- The minimum source-compatibility line is the latest Go 1.23 patch.
- CI tests Go 1.23 compatibility plus the security-supported Go 1.25.12 and
  Go 1.26.5 toolchains.
- The supported and tested GORM line begins at v1.31.2.
- SQLite, PostgreSQL, and MySQL GORM drivers are v1.6.0.

The release module pins GORM v1.31.2 and the three v1.6.0 drivers. A newer
patch/minor is supported only after the same test, race, real-dialect, and API
campaign passes and the compatibility table is updated. Dependency automation
proposes updates; it does not silently expand the support claim.

Go 1.23 is retained as a compile/source compatibility lane but is no longer a
security-supported upstream Go line. Production binaries must use a currently
supported Go release with all security patches; for this release that means
Go 1.25.12 or Go 1.26.5. The release vulnerability scan is pinned to Go 1.26.5.
This distinction lets libraries with an older module baseline compile while
preventing an end-of-life standard library from being represented as safe for
deployment.
The declared source-compatibility range ends at Go 1.26.5 until a newer
toolchain completes CI; future Go releases are not silently claimed.

## Database dialects

The conformance suite covers:

- embedded SQLite 3.45.1 through `gorm.io/driver/sqlite` v1.6.0 and
  `go-sqlite3` v1.14.22;
- PostgreSQL 17.x in CI;
- MySQL 8.4.x LTS in CI.

DryRun quoting, placeholders, bound variables, explicit null ordering, and
cursor keyset predicates are tested
for all three on every ordinary test run. Real PostgreSQL/MySQL list, count,
tenant-scope, offset/cursor pagination, nested relationships, many-to-many,
composite keys, nullable sorts, and soft-delete behavior runs in a dedicated CI
job and again in the release workflow.

Cursor payload v1 and `NULLS LAST` ordering are cross-dialect protocol
contracts. Patch releases cannot reinterpret an existing cursor version.

Other database versions may work through GORM but are not claimed until added
to the conformance matrix. Unsupported behavior must fail during engine
construction or be documented before release; it must not silently generate a
differently interpreted query.

`TestReleaseDependencySupportMatrix` checks the module's GORM and driver
versions. CI runs the real-dialect suite with PostgreSQL 17 and MySQL 8.4.

## Stable compatibility

Stable v1 freezes exported Go API, query grammar, canonical parameter behavior,
error codes, cursor payload v1, and serialized response/description fields.
Stable changes follow Semantic Versioning and the deprecation windows in
`docs/api-policy.md`. Every release compares its complete exported module API
with the previous reachable tag and requires reviewed classification.
