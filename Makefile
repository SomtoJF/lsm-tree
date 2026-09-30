run:
	CompileDaemon -build="go build -o lsm-tree" -command="./lsm-tree"

test:
	go test -v ./...

BENCHTIME ?= 1s
COUNT ?= 1

bench-db:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabase' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

bench-track:
	@{ \
		printf '\n### Test Run: %s (benchtime=%s, count=%s)\n\n```text\n' "$$(date '+%Y-%m-%d %H:%M:%S')" "$(BENCHTIME)" "$(COUNT)"; \
		go test ./database/tests -run '^$$' -bench '^BenchmarkDatabase' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem; \
		status=$$?; \
		printf '\n```\n'; \
		exit $$status; \
	} >> BENCHMARKS.md

bench-db-set:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabaseSet$$' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

bench-db-get:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabaseGet$$' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

bench-db-parallel:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabaseParallelGet$$' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

bench-db-parallel-set:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabaseParallelSet$$' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

bench-db-readwrite:
	go test ./database/tests -run '^$$' -bench '^BenchmarkDatabaseConcurrentReadWrite$$' -benchtime=$(BENCHTIME) -count=$(COUNT) -benchmem

test-bench: bench-db

.PHONY: bench-db bench-track

build:
	go build -o lsm-tree

clean:
	rm -f lsm-tree