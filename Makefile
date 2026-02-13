BINARY=lazy-tmux

.PHONY: build test fmt tidy install clean

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

install:
	go install ./cmd/$(BINARY)

clean:
	rm -rf bin dist
