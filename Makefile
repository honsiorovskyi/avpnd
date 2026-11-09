# Copyright (C) 2026 honsiorovskyi
# SPDX-License-Identifier: AGPL-3.0-only

# Version comes from the latest git tag (vX.Y.Z), e.g. v0.2.0 or v0.2.0-3-gabc1234-dirty.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: release version bump build test install vet fmt check clean help extensions snapshot

help:
	@echo "avpnd - AWS VPN Client wrapper"
	@echo ""
	@echo "Targets:"
	@echo "  build   - Build the avpnd binary"
	@echo "  version - Print the current version (from git tags)"
	@echo "  release - Test, bump (patch) and push if there are changes since the last tag"
	@echo "  bump    - Tag the next version: make bump BUMP=patch|minor|major"
	@echo "  test    - Run tests"
	@echo "  check   - Run vet, tests and build"
	@echo "  extensions - Build the Firefox and Chrome extension zips into build/extensions"
	@echo "  snapshot   - Local goreleaser dry run (needs goreleaser)"
	@echo "  vet     - Run go vet"
	@echo "  install - Install avpnd to GOBIN"
	@echo "  clean   - Remove build artifacts"

version:
	@echo $(VERSION)

# Release changes since the last tag: tests, bump (patch by default), push commits and tag.
# make release [BUMP=minor|major] [DRY_RUN=1]
release:
	scripts/release.sh $(BUMP)

# Tag the next semantic version: make bump BUMP=patch|minor|major (default patch).
# Creates the tag locally; push it to publish a release.
bump:
	scripts/bump.sh $(BUMP)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o avpnd

test:
	go test -race ./...

vet:
	go vet ./...

install: build
	go install

clean:
	rm -f avpnd
	rm -rf build dist
	go clean

fmt:
	gofmt -l .

extensions:
	scripts/build-extensions.sh $(if $(filter command line,$(origin VERSION)),$(VERSION))

snapshot:
	goreleaser release --snapshot --clean

check: vet test build
	@echo "All checks passed!"

.DEFAULT_GOAL := help
