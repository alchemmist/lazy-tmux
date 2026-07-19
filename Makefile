.DEFAULT_GOAL := check

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-variables

BINARY := lazy-tmux

.PHONY: check build build-fzf build-all test test-cov integration-test fmt install clean dist dist-tui dist-fzf release-patch release-minor release-major sandbox test-sup-versions docker-hub vet setup-env golangci-lint docs-install docs-dev docs-build docs-preview gifs

check: build vet golangci-lint test integration-test

integration-test:
	podman build -f ./docker/test.Dockerfile -t lazy-tmux-test . && podman run --rm lazy-tmux-test

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
	podman build -f ./docker/test.Dockerfile -t lazy-tmux-test .
	podman rm -f lazy-tmux-cov 2>/dev/null || true
	podman run --name lazy-tmux-cov -e GOCACHE=/tmp/gocache -e GOFLAGS=-buildvcs=false lazy-tmux-test \
	go test -p 1 -coverprofile=cover.out -covermode=atomic -coverpkg=$$(go list ./... | grep -v /internal/testutil | paste -sd "," -) ./...
	podman cp lazy-tmux-cov:/workspace/cover.out ./cover.out
	podman rm -f lazy-tmux-cov
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

# Cut a release: guards (main, clean, synced), `make check`, semver tag, push —
# the release workflow (GoReleaser, Homebrew tap, AUR) picks the tag up from
# there. Shows the commits being released and asks for confirmation.
release-patch:
	./scripts/release.sh patch

release-minor:
	./scripts/release.sh minor

release-major:
	./scripts/release.sh major

setup-env:
	go install gotest.tools/gotestsum@v1.13.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/vladopajic/go-test-coverage/v2@v2.18.4

docker-hub:
	SANDBOX_TAG=$${SANDBOX_TAG:-latest}; \
	podman build -t lazy-tmux:$$SANDBOX_TAG -f docker/sandbox.Dockerfile .; \
	podman tag lazy-tmux:$$SANDBOX_TAG alchemmist/lazy-tmux:$$SANDBOX_TAG; \
	podman push alchemmist/lazy-tmux:$$SANDBOX_TAG

# Build and drop into the interactive sandbox. The sandbox installs the
# lazy-tmux binary built from the current working tree (including uncommitted
# changes), not the last release, so you test the code in front of you. Pick the
# tmux version with TMUX_VERSION (defaults to 3.6a); each version gets its own
# cached image tag.
#   make sandbox                  # tmux 3.6a
#   make sandbox TMUX_VERSION=3.5a
TMUX_VERSION ?= 3.6a
sandbox:
	CGO_ENABLED=0 GOOS=linux GOARCH=$$(go env GOARCH) go build -o docker/local-bin/$(BINARY) ./cmd/$(BINARY)
	podman build --build-arg TMUX_VERSION=$(TMUX_VERSION) --build-arg LAZY_TMUX_SOURCE=local -t lazy-tmux:local-$(TMUX_VERSION) -f docker/sandbox.Dockerfile .
	podman run -it --rm lazy-tmux:local-$(TMUX_VERSION)

test-sup-versions:
	@chmod +x scripts/test-tmux-versions.sh
	@./scripts/test-tmux-versions.sh

# ---- docs site (Vite + React + Gravity UI, in docs/) ----
# The site is statically pre-rendered with vite-react-ssg; CI (pages.yml) runs
# the same npm steps. install.sh, CNAME and assets/ live in docs/public.
docs-install:
	npm --prefix docs ci

docs-dev:
	npm --prefix docs run dev

docs-build:
	npm --prefix docs run build

docs-preview: docs-build
	npm --prefix docs run preview

gifs:
	CGO_ENABLED=0 GOOS=linux GOARCH=$$(go env GOARCH) go build -o docs/tapes/$(BINARY) ./cmd/$(BINARY)
	podman build -t lazy-tmux-vhs docs/tapes
	@for tape in docs/tapes/*.tape; do \
		echo "==> recording $$tape"; \
		podman run --rm \
			-v "$(CURDIR)/docs/public/assets:/root/out" \
			-v "$(CURDIR)/$$tape:/root/tape.tape:ro" \
			lazy-tmux-vhs /root/tape.tape || exit 1; \
	done

clean:
	rm -rf bin dist coverage.out cover.html cover.out .cache docker/local-bin/$(BINARY) docs/tapes/$(BINARY)
