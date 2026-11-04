BINARIES := kagi-search kagi-extract

.PHONY: build test test-race lint fmt fmt-check vet clean check

# Each binary is built into its skill directory's .bin/, where the wrapper
# scripts look for it (<skill-dir>/.bin/<name>).
build:
	@set -e; \
	for b in $(BINARIES); do \
		echo "=== $$b ==="; \
		go build -o $$b/.bin/$$b ./cmd/$$b; \
	done

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run --config .golangci.yml ./...

fmt:
	gofumpt -w -l .

# Fails (and lists the files) if any file needs formatting.
fmt-check:
	@files=$$(gofumpt -l .); \
	if [ -n "$$files" ]; then \
		echo "Files need formatting (run: make fmt):"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

clean:
	rm -rf $(addsuffix /.bin,$(BINARIES)) dist

check: fmt-check vet test lint
