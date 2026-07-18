# Contributing

Contributions are welcome. Open an issue before a large API or query-language
change so the design can be discussed before implementation.

## Development

The core module requires Go 1.23 or newer:

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -w .
go mod tidy
```

The documentation site is a separate Node project:

```bash
cd website
npm ci
npm run build
```

Framework and production examples use separate Go modules so their dependencies
do not leak into the library:

```bash
cd examples/frameworks && go test ./...
cd ../production && go test ./...
```

## Pull requests

- Add tests for behavior changes and regressions.
- Keep client values in GORM bound variables; never interpolate them into SQL.
- Do not expose a model field or relationship without an explicit policy.
- Update public documentation and conformance fixtures when query behavior
  changes.
- Keep error handling compatible with the registry in `docs/error-policy.md`.
- Run the tests, race detector, vet, and documentation build before submitting.

The exported v1 API is checked against the latest release tag by
`scripts/check-api-compat.sh`. Public API changes require explicit maintainer
review and an update to that compatibility gate.
