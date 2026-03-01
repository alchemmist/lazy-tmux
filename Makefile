.DEFAULT_GOAL := help

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

BINARY := lazy-tmux
GO_PACKAGES := ./...
GOFMT_PATHS := ./cmd ./internal
GOBIN := $(shell go env GOPATH)/bin
STATICCHECK := $(GOBIN)/staticcheck
GOLANGCI_LINT := $(GOBIN)/golangci-lint

.PHONY: help check build test test-race test-cov fmt fmt-check vet staticcheck golangci-lint lint tidy tidy-check install clean

check: fmt-check tidy-check lint staticcheck test build

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test $(GO_PACKAGES)

test-race:
	go test -race $(GO_PACKAGES)

test-cov: 
	go install github.com/vladopajic/go-test-coverage/v2@latest
	go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...
	${GOBIN}/go-test-coverage --config=./.testcoverage.yml

fmt:
	gofmt -w $(GOFMT_PATHS)

fmt-check:
	@unformatted="$$(gofmt -l $(GOFMT_PATHS))"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet $(GO_PACKAGES)

staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	$(STATICCHECK) $(GO_PACKAGES)

golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOLANGCI_LINT) run ./...

lint: vet staticcheck golangci-lint

tidy:
	go mod tidy

tidy-check:
	go mod tidy
	@git diff --exit-code -- go.mod go.sum

install:
	go install ./cmd/$(BINARY)

clean:
	rm -rf bin dist coverage.out

help:
	@printf "Available targets:\n\n"
	@printf "  check       - run fmt-check, tidy-check, lint, test, build\n"
	@printf "  build       - build binary into ./bin\n"
	@printf "  test        - run all tests\n"
	@printf "  test-race   - run tests with race detector\n"
	@printf "  cover       - run tests with coverage profile\n"
	@printf "  fmt         - format Go sources with gofmt\n"
	@printf "  fmt-check   - verify Go sources are formatted\n"
	@printf "  vet         - run go vet\n"
	@printf "  staticcheck - run staticcheck\n"
	@printf "  golangci-lint - run golangci-lint\n"
	@printf "  lint        - run vet + staticcheck + golangci-lint\n"
	@printf "  tidy        - run go mod tidy\n"
	@printf "  tidy-check  - ensure go.mod/go.sum are tidy\n"
	@printf "  install     - install CLI with go install\n"
	@printf "  clean       - remove build artifacts\n"
