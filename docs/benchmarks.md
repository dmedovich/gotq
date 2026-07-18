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
