# Benchmark baseline

Status: stable v1.0 release baseline
Date: 2026-07-16

Benchmarks help detect regressions; they are not portable performance promises.
Compare results on the same host, Go version, CPU governor, GORM version, and
database configuration with `benchstat`.

Baseline command:

```bash
go test -run='^$' -bench=. -benchmem -benchtime=200ms -count=1 ./...
```

Local reference environment:

- Linux amd64;
- AMD Ryzen 5 2600;
- Go 1.26.5;
- in-process GORM DryRun for parse/apply benchmarks;
- single-connection in-memory SQLite for list execution.

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkParseHTTP` | 2,567 | 1,744 | 13 |
| `BenchmarkParseMaximumValidFilter` | 53,611 | 76,608 | 158 |
| `BenchmarkRejectOversizedFilter` | 5,814 | 336 | 5 |
| `BenchmarkApply` | 4,711 | 3,152 | 41 |
| `BenchmarkEngineParseApply` | 9,215 | 6,368 | 64 |
| `BenchmarkEngineNewSchemaCacheHit` | 8,046 | 3,192 | 49 |
| `BenchmarkEngineListSQLite` | 345,911 | 31,489 | 699 |
| `BenchmarkRelationshipParseAndApply` | 15,533 | 9,135 | 83 |
| `BenchmarkCursorParseAndApply` | 21,693 | 14,106 | 102 |

Nightly CI records five samples as an artifact. Timing is initially advisory.
An unexplained allocation increase or algorithmic-complexity regression blocks
release review even when shared-runner timing is noisy.

## v1.1.0 cursor scan comparison

Measured on 2026-10-18, Linux amd64 / Ryzen 5 2600 / Go 1.27.2, with 1,000
SQLite rows on one connection. `BenchmarkCursorPage` measures continuation
pages of 1, 25, and 100 rows, using primary-key, duplicate scalar, and nullable
relationship sorts. Both samples include default-sort and observation support
with observation disabled. The only comparison change is extracting cursor
keys from the last returned row instead of boxing keys for every result row.

```bash
go test -run='^$' -bench='^BenchmarkCursorPage$' -benchmem -benchtime=100ms -count=6
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e before.txt after.txt
```

| 100-row continuation | Allocations before → after | Allocation reduction | Memory reduction |
| --- | ---: | ---: | ---: |
| Primary key | 1,582 → 1,381 | 12.7% | 7.7% |
| Duplicate name | ~2,175 → ~1,873 | 13.9% | 10.3% |
| Nullable relationship name | ~2,627 → ~2,325 | 11.5% | 9.2% |

All nine cases allocate less memory. Timing is noisy on this shared host and
is not a performance guarantee; the decision is supported by the consistent
allocation reduction. No sort-plan cache or unbounded client-controlled cache
was introduced. Lookahead rows, model hooks, projection handling, and cursor
payload v1 semantics are retained. Terminal pages need no cursor-key extraction.

Raw evidence: [before](benchmarks/v1.1.0/cursor-before.txt),
[after](benchmarks/v1.1.0/cursor-after.txt), and
[benchstat comparison](benchmarks/v1.1.0/cursor-benchstat.txt).
