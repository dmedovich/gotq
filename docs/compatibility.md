# Compatibility policy

Status: v1 compatibility contract; current dependency validation targets
Updated: 2026-10-18

## Go and GORM

- The minimum Go version declared by the library is Go 1.25.0.
- CI targets Go 1.25.14 for source compatibility and Go 1.26.9 / Go 1.27.2
  for supported production toolchains.
- The supported and tested GORM line begins at v1.31.2.
- SQLite and MySQL GORM drivers are v1.6.0; PostgreSQL is v1.6.3.

The module pins GORM v1.31.2 and the driver versions above. A newer
patch/minor is supported only after the same test, race, real-dialect, and API
campaign passes and the compatibility table is updated. Dependency automation
proposes updates; it does not silently expand the support claim.

Go 1.25 is a source-compatibility lane, not a security-supported production
toolchain. Production binaries must use a currently supported Go release with
all security patches. The release vulnerability scan uses Go 1.27.2.
See the [upstream release policy and security fixes](https://go.dev/doc/devel/release).
These are validation targets for the current branch; a release support claim
requires its complete CI campaign to pass. Future toolchains are not silently
claimed.

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
