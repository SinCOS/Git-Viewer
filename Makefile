GO := $(if $(wildcard .tools/go/bin/go),$(CURDIR)/.tools/go/bin/go,go)
export PATH := $(CURDIR)/.tools/go/bin:$(PATH)

.PHONY: check fmt lint test build run smoke package
check: fmt lint test build
fmt:
	@test -z "$$($(GO) fmt ./...)" || (echo 'Go formatting changed; review and rerun'; exit 1)
lint:
	$(GO) vet ./...
	$(GO) tool mygo vet
test:
	$(GO) test -race -count=1 ./...
build:
	CGO_ENABLED=0 $(GO) build -o bin/git-viewer ./cmd/git-viewer
run:
	$(GO) run ./cmd/git-viewer
smoke: build
	./bin/git-viewer -smoke
package:
	$(GO) tool mygo build -platform darwin/amd64,darwin/arm64 -skip-dmg
