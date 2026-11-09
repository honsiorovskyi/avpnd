.PHONY: build test install vet fmt check clean help extensions snapshot

help:
	@echo "avpnd - AWS VPN Client wrapper"
	@echo ""
	@echo "Targets:"
	@echo "  build   - Build the avpnd binary"
	@echo "  test    - Run tests"
	@echo "  check   - Run vet, tests and build"
	@echo "  extensions - Build the Firefox and Chrome extension zips into build/extensions"
	@echo "  snapshot   - Local goreleaser dry run (needs goreleaser)"
	@echo "  vet     - Run go vet"
	@echo "  install - Install avpnd to GOBIN"
	@echo "  clean   - Remove build artifacts"

build:
	go build -o avpnd

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
	scripts/build-extensions.sh $(VERSION)

snapshot:
	goreleaser release --snapshot --clean

check: vet test build
	@echo "All checks passed!"

.DEFAULT_GOAL := help
