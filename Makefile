# python-markdown2 Makefile (Go port)
SHELL=/bin/bash

.PHONY: all
all: build

.PHONY: build
build:
	go build -o markdown2 ./cmd/markdown2

.PHONY: test
test:
	go vet ./...
	go test ./...

# The test suite without the slow ReDoS checks.
.PHONY: testone
testone:
	go test -short ./...

.PHONY: testredos
testredos:
	go test -run TestReDoS -v .

.PHONY: bench
bench:
	go test -run XXX -bench . .

clean:
	rm -rf build dist markdown2

.PHONY: versioncheck
versioncheck:
	[[ `grep '^const Version' markdown2.go | cut -d'"' -f2` \
	    == `grep '^## ' CHANGES.md | head -1 | awk '{print $$3}'` ]]
	@echo Version check ok.
