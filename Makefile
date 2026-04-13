.DEFAULT_GOAL := check

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-variables

BINARY := lazy-tmux

.PHONY: check build build-fzf build-all test test-cov integration-test fmt install clean dist dist-tui dist-fzf tag sandbox test-sup-versions docker-hub vet setup-env golangci-lint

check: build vet golangci-lint test integration-test

integration-test:
	docker build -f ./docker/test.Dockerfile -t lazy-tmux-test . && docker run --rm lazy-tmux-test

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

fmt:
	golangci-lint fmt
	golangci-lint run --fix --issues-exit-code=0 --output.text.path=/dev/null --show-stats=false

build-fzf:
	go build -tags lazy_fzf -o bin/$(BINARY)-fzf ./cmd/$(BINARY)

build-all: build build-fzf

test:
	gotestsum --format testname -- -race ./...

test-cov:
	docker build -f ./docker/test.Dockerfile -t lazy-tmux-test . && \
	docker run --rm --user $$(id -u):$$(id -g) -v $$(pwd):/workspace -w /workspace -e GOCACHE=/workspace/.cache lazy-tmux-test \
	go test -p 1 -coverprofile=cover.out -covermode=atomic -coverpkg=$$(go list ./... | grep -v /internal/testutil | paste -sd "," -) ./...
	go tool cover -html=cover.out -o cover.html || true
	go-test-coverage --config=./.testcoverage.yml

vet:
	go vet ./...

golangci-lint:
	golangci-lint run

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

setup-env:
	go install gotest.tools/gotestsum@v1.13.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/vladopajic/go-test-coverage/v2@v2.18.4

docker-hub:
	SANDBOX_TAG=$${SANDBOX_TAG:-latest}; \
	docker build -t lazy-tmux:$$SANDBOX_TAG -f docker/sandbox.Dockerfile .; \
	docker tag lazy-tmux:$$SANDBOX_TAG alchemmist/lazy-tmux:$$SANDBOX_TAG; \
	docker push alchemmist/lazy-tmux:$$SANDBOX_TAG

sandbox:
	docker build -t lazy-tmux:local -f docker/sandbox.Dockerfile .
	docker run -it --rm lazy-tmux:local

test-sup-versions:
	chmod +x scripts/test-tmux-versions.sh
	./scripts/test-tmux-versions.sh

clean:
	rm -rf bin dist coverage.out cover.html cover.out .cache
