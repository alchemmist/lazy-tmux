BINARY=lazy-tmux

.PHONY: all ci build test fmt fmt-check vet staticcheck golangci-lint lint tidy install clean

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@unformatted="$$(gofmt -l ./cmd ./internal)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	$$(go env GOPATH)/bin/staticcheck ./...

golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$$(go env GOPATH)/bin/golangci-lint run ./...

lint: vet staticcheck golangci-lint

ci: fmt-check lint test build

tidy:
	go mod tidy

install:
	go install ./cmd/$(BINARY)

clean:
	rm -rf bin dist
