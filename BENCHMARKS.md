# Benchmark Results

```shell
=== Test Run: 2026-09-30 15:52:28 ===
go test ./database/tests -run '^$' -bench '^BenchmarkDatabase' -benchtime=1s -count=1 -benchmem
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                   	   23088	     51514 ns/op	    1467 B/op	       8 allocs/op
BenchmarkDatabaseGet-8                   	   85839	     13561 ns/op	     292 B/op	       6 allocs/op
BenchmarkDatabaseParallelGet-8           	   80354	     14763 ns/op	     292 B/op	       6 allocs/op
BenchmarkDatabaseParallelSet-8           	   24681	     49690 ns/op	    1461 B/op	       8 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8   	   28372	     39389 ns/op	     979 B/op	       7 allocs/op
PASS
ok  	github.com/SomtoJF/lsm-tree/database/tests	11.598s
```
### Test Run: 2026-09-30 15:55:06
### Test Run: 2026-09-30 15:55:23

### Test Run: 2026-09-30 15:56:31 (benchtime=100ms, count=1)

```text
goos: darwin
goarch: arm64
pkg: github.com/SomtoJF/lsm-tree/database/tests
cpu: Apple M1
BenchmarkDatabaseSet-8                   	    2698	     50826 ns/op	    1455 B/op	       7 allocs/op
BenchmarkDatabaseGet-8                   	    8308	     14897 ns/op	     292 B/op	       6 allocs/op
BenchmarkDatabaseParallelGet-8           	    6637	     16041 ns/op	     294 B/op	       6 allocs/op
BenchmarkDatabaseParallelSet-8           	    2676	     50024 ns/op	    1461 B/op	       7 allocs/op
BenchmarkDatabaseConcurrentReadWrite-8   	    1563	     63996 ns/op	    1796 B/op	      12 allocs/op
PASS
ok  	github.com/SomtoJF/lsm-tree/database/tests	2.471s

```
