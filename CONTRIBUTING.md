# Contributing

Contributions are welcome. Open an issue before a large API or query-language
change so the design can be discussed before implementation.

## Development

The core module requires Go 1.25 or newer:

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
npm ci --ignore-scripts
npm run audit:ci
npm run build
```

Framework and production examples use separate Go modules so their dependencies
do not leak into the library:

```bash
cd examples/frameworks && go test ./...
cd ../production && go test ./...
```

After updating core dependencies, run `go mod tidy` in the root module and in
both example modules. Their local `replace` directives use the core dependency
graph, but their `go.sum` files are independent. Run `go mod tidy -diff` in each
module before submitting.

GORM or driver updates also require reviewing `TestReleaseDependencySupportMatrix`
and the compatibility pages in `docs/` and `website/docs/`. Keep the exact-version
gate: update its expectations only after checking dialect conformance.
CI toolchains are pinned in the CI, nightly, and release workflows; update those
pins together when Go security fixes ship.

## Pull requests

- Add tests for behavior changes and regressions.
- Keep client values in GORM bound variables; never interpolate them into SQL.
- Do not expose a model field or relationship without an explicit policy.
- Update public documentation and conformance fixtures when query behavior
  changes.
- Keep error handling compatible with the registry in `docs/error-policy.md`.
- Run the tests, race detector, vet, and documentation build before submitting.

The exported v1 API is checked against the latest release tag by
`scripts/check-api-compat.sh`. Install the same tool version used in CI:

```bash
go install golang.org/x/exp/cmd/apidiff@v0.0.0-20250620022241-b7579e27df2b
scripts/test-api-compat.sh
scripts/check-api-compat.sh
```

For an intentional compatible addition in a minor release, run
`scripts/check-api-compat.sh --write-review`, review the generated
`docs/api-diffs/<baseline-tag>.txt`, and include it in the PR. This records the
exact delta and baseline commit; it is not a general permission for further
changes. Ordinary CI only reads the record. Incompatible changes fail even
when writing a review record. Use `GOTQ_API_BASE_REF=v1.0.0` to select an
explicit baseline. Full Git history and release tags are required.
