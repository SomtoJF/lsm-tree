run:
	CompileDaemon -build="go build -o lsm-tree" -command="./lsm-tree"

test:
	go test -v ./...

test-bench:
	go test -bench=. -benchmem -v ./...

build:
	go build -o lsm-tree

clean:
	rm -f lsm-tree