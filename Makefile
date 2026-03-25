.DEF.DEFAULT_GOAL := help

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

BINARY := lazy-tmux

.PHONY: help check build build-fzf build-all test test-race test-cov test-integration fmt fmt-check vet staticcheck golangci-lint lint tidy install clean dist dist-tui dist-fzf tag

check: fmt-check lint test build

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

build-fzf:
	go build -tags lazy_fzf -o bin/$(BINARY)-fzf ./cmd/$(BINARY)

build-all: build build-fzf

test:
	go install gotest.tools/gotestsum@latest
	gotestsum -- ./...

test-race:
	go install gotest.tools/gotestsum@latest
	gotestsum -- -race ./...

test-cov:
	go install gotest.tools/gotestsum@latest
	go install github.com/vladopajic/go-test-coverage/v2@latest
	gotestsum -- -coverprofile=cover.out -covermode=atomic -coverpkg=./... ./...
	go-test-coverage --config=./.testcoverage.yml

test-integration:
	docker build -f docker/integration.Dockerfile -t lazy-tmux-integration .
	docker run --rm lazy-tmux-integration

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
	$(shell go env GOPATH)/bin/staticcheck ./...

golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(shell go env GOPATH)/bin/golangci-lint run ./...

lint: vet staticcheck golangci-lint

tidy:
	go mod tidy

install: build
	go install ./cmd/$(BINARY)

dist:
	goreleaser release --snapshot --clean

dist-tui:
	goreleaser release --snapshot --clean --id lazy-tmux

dist-fzf:
	goreleaser release --snapshot --clean --id lazy-tmux-fzf

tag:
	@if ! git diff --quiet || ! git diff --cached --quiet; then \
		echo "working tree is dirty; commit or stash changes first"; \
		exit 1; \
	fi
	@latest="$$(git tag --list 'v*' --sort=-v:refname | head -n1)"; \
	if [ -z "$$latest" ]; then \
		next="v0.1.0"; \
	else \
		ver="$${latest#v}"; \
		IFS=. read -r major minor patch <<< "$$ver"; \
		case "$${TYPE:-patch}" in \
			patch) patch=$$((patch+1));; \
			minor) minor=$$((minor+1)); patch=0;; \
			major) major=$$((major+1)); minor=0; patch=0;; \
			*) echo "TYPE must be patch, minor, or major"; exit 1;; \
		esac; \
		next="v$${major}.$${minor}.$${patch}"; \
	fi; \
	if git rev-parse -q --verify "refs/tags/$$next" >/dev/null; then \
		echo "tag $$next already exists"; \
		exit 1; \
	fi; \
	echo "tagging $$next"; \
	git tag -a "$$next" -m "release $$next"

clean:
	rm -rf bin dist coverage.out

help:
	@printf "Available targets:\n\n"
	@printf "  check       - run fmt-check, lint, test, build\n"
	@printf "  build       - build binary into ./bin\n"
	@printf "  build-fzf   - build fzf-only binary into ./bin\n"
	@printf "  build-all   - build both tui and fzf-only binaries into ./bin\n"
	@printf "  test        - run all tests\n"
	@printf "  test-race   - run tests with race detector\n"
	@printf "  cover       - run tests with coverage profile\n"
	@printf "  test-integration - run integration tests (tmux + TUI)\n"
	@printf "  fmt         - format Go sources with gofmt\n"
	@printf "  fmt-check   - verify Go sources are formatted\n"
	@printf "  vet         - run go vet\n"
	@printf "  staticcheck - run staticcheck\n"
	@printf "  golangci-lint - run golangci-lint\n"
	@printf "  lint        - run vet + staticcheck + golangci-lint\n"
	@printf "  tidy        - run go mod tidy\n"
	@printf "  install     - install CLI with go install\n"
	@printf "  dist        - build all release artifacts locally (snapshot)\n"
	@printf "  dist-tui    - build tui artifacts locally (snapshot)\n"
	@printf "  dist-fzf    - build fzf artifacts locally (snapshot)\n"
	@printf "  tag         - create next git tag (TYPE=patch|minor|major)\n"
	@printf "  clean       - remove build artifacts\n"printf "  clean       - remove build artifacts\n"
