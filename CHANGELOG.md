# Changelog

Notable changes to gotq are documented here. The project follows Semantic
Versioning.

## Unreleased

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
