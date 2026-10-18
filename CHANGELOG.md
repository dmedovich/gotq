# Changelog

Notable changes to gotq are documented here. The project follows Semantic
Versioning.

## 1.1.0 — 2026-10-18

### Added

- Startup-validated `Config.DefaultSort`, used when the client omits sort and
  exposed in endpoint descriptions and OpenAPI. Explicit client sorts replace
  the default; cursor v1 and primary-key/null ordering remain unchanged.
- Opt-in `Config.Observe` callbacks for List parse, validation, count, and data
  phase durations and stable error codes, without exposing query contents.
- Exact reviewed API deltas for compatible additions, while always rejecting
  incompatible API changes; integration tests cover the gate itself.

### Fixed

- Engine/session `Apply` now enforces receiving-endpoint limits for queries
  parsed elsewhere or manually constructed; stricter parser limits are retained.
- OpenAPI includes `filter` for policies whose only filterable fields are in
  to-many relationships, with quantifier guidance and nested policy metadata.

### Performance

- Extract next-cursor keys only for the last returned row. In the recorded
  100-row continuation benchmarks this reduces allocations by 11–14% and memory
  by 8–10%; see `docs/benchmarks.md` for methodology and raw measurements.

### Maintenance

- Align the PostgreSQL driver compatibility check with v1.6.3 and refresh both
  example modules' checksums after core dependency updates.
- Update CI, nightly, and release toolchains to Go 1.27.2; retain a Go 1.25.14
  compatibility lane and add Go 1.26.9 coverage.
- Correct the documented minimum Go version to 1.25 and add dependency
  automation for production examples.
- Refresh documentation-site dependencies, including patched selector parsing
  and worker-pool dependencies.
- Accept only `GHSA-vfj7-8cjw-p6xm` for the documentation build's `braces` 3.0.3
  until 2026-11-18. The dependency remains unpatched; all other moderate-or-higher
  advisories and audit failures still block CI. See `SECURITY.md` for the scope
  and exposure assessment.

## 1.0.0

Initial public release.

### Added

- Schema-validated filtering, sorting, search, count, and bounded pagination
  for GORM list endpoints.
- Scalar operators, sets, null predicates, date/UUID/decimal values, and
  explicit custom codecs.
- Opt-in to-one relationships and to-many `any`/`all` filters.
- Stable offset and forward-cursor pagination with primary-key tie-breakers.
- Structured errors, endpoint descriptions, OpenAPI generation, framework
  examples, and a parser playground.
- SQLite, PostgreSQL, and MySQL support.
