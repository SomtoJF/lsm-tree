# Benchmark Results

## Background Compaction

### Before — 2026-11-13 02:09 (synchronous compaction; previous commit)

```text
BenchmarkDatabaseSet-8           56  11507365 ns/op   979 B/op   12 allocs/op
BenchmarkDatabaseParallelSet-8  308   2096078 ns/op  1022 B/op   12 allocs/op
```

Notes:

- Foreground writes included compaction work while holding the database mutex.
- Set: ~86.9 ops/s; ParallelSet: ~477.1 ops/s.

### After — 2026-11-13 02:10 (background compaction)

```text
BenchmarkDatabaseSet-8           50  10270598 ns/op  1260 B/op   16 allocs/op
BenchmarkDatabaseParallelSet-8  339   1742713 ns/op  1054 B/op   13 allocs/op
```

Outcome:

- Set improved ~10.7% to ~97.4 ops/s.
- ParallelSet improved ~16.9% to ~573.8 ops/s.
- Compaction no longer blocks foreground writes; `fsync` and the 5 ms commit window remain the dominant write costs.

## Crash Recovery

### Before — 2026-11-13 00:19:48 (pre-crash-recovery baseline; last run of count=5)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                  92586          10894 ns/op     406 B/op     4 allocs/op
BenchmarkDatabaseGet-8                  89582          13365 ns/op     287 B/op     6 allocs/op
BenchmarkDatabaseParallelGet-8          83360          14560 ns/op     287 B/op     6 allocs/op
BenchmarkDatabaseParallelSet-8          90481          12203 ns/op     409 B/op     4 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8  69884          15355 ns/op     378 B/op     5 allocs/op
PASS
```

Notes:

- `BenchmarkDatabaseSet`: ~10.9 us/op (~92k ops/s).
- `BenchmarkDatabaseGet`: ~13.4 us/op.
- `BenchmarkDatabaseParallelGet`: ~14.6 us/op.
- `BenchmarkDatabaseParallelSet`: ~12.2 us/op (~82k ops/s).
- `BenchmarkDatabaseConcurrentReadWrite`: ~15.4 us/op.

### After — 2026-11-13 00:41:44 (crash recovery)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                    124       9719773 ns/op    1316 B/op    14 allocs/op
BenchmarkDatabaseGet-8                  85032         14224 ns/op     288 B/op     6 allocs/op
BenchmarkDatabaseParallelGet-8          76206         17772 ns/op     289 B/op     6 allocs/op
BenchmarkDatabaseParallelSet-8            655       1930308 ns/op    1127 B/op    13 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8   1082       1045897 ns/op     808 B/op    10 allocs/op
PASS
```

Outcome:

- Reads stayed roughly stable: `Get` changed from ~13.4 us/op to ~14.2 us/op.
- Writes became slower because each successful write waits for durable syncing: `Set` changed from ~92k ops/s to ~103 ops/s.
- Parallel writes improved batching relative to sequential writes, but remained much slower overall: ~82k ops/s to ~518 ops/s.
- Mixed read/write performance dropped because writes now wait for durability.

### Test Run: 2026-10-09 10:29:45 (benchtime=1s, count=1)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                  116638          10136 ns/op     476 B/op     4 allocs/op
BenchmarkDatabaseGet-8                   94988          12754 ns/op     287 B/op     6 allocs/op
BenchmarkDatabaseParallelGet-8           82857          14251 ns/op     287 B/op     6 allocs/op
BenchmarkDatabaseParallelSet-8          117972          11409 ns/op     475 B/op     4 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8   83337          13891 ns/op     366 B/op     5 allocs/op
PASS
```

### Test Run: 2026-09-30 15:56:31 (benchtime=100ms, count=1)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                    2698          50826 ns/op    1455 B/op     7 allocs/op
BenchmarkDatabaseGet-8                    8308          14897 ns/op     292 B/op     6 allocs/op
BenchmarkDatabaseParallelGet-8            6637          16041 ns/op     294 B/op     6 allocs/op
BenchmarkDatabaseParallelSet-8            2676          50024 ns/op    1461 B/op     7 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8    1563          63996 ns/op    1796 B/op    12 allocs/op
PASS
```

### Test Run: 2026-09-30 15:52:28 (benchtime=1s, count=1)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                   23088          51514 ns/op    1467 B/op     8 allocs/op
BenchmarkDatabaseGet-8                   85839          13561 ns/op     292 B/op     6 allocs/op
BenchmarkDatabaseParallelGet-8           80354          14763 ns/op     292 B/op     6 allocs/op
BenchmarkDatabaseParallelSet-8           24681          49690 ns/op    1461 B/op     8 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8   28372          39389 ns/op     979 B/op     7 allocs/op
PASS
```
