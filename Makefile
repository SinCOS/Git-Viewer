GO := $(if $(wildcard .tools/go/bin/go),$(CURDIR)/.tools/go/bin/go,go)
export PATH := $(CURDIR)/.tools/go/bin:$(PATH)

.PHONY: check fmt lint test build native run smoke package dialog-smoke helper helper-clean
check: fmt lint test build
fmt:
	@test -z "$$($(GO) fmt ./...)" || (echo 'Go formatting changed; review and rerun'; exit 1)
lint:
	$(GO) vet ./...
	$(GO) tool mygo vet
test:
	$(GO) test -race -count=1 ./...
helper:
	@set -e; for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do \
		mkdir -p "resources/helper/$$target"; \
		CGO_ENABLED=0 GOOS=$${target%-*} GOARCH=$${target#*-} $(GO) build -o "resources/helper/$$target/gitviewer-helper" ./cmd/gitviewer-helper; \
	done
helper-clean:
	rm -rf resources/helper
build: native helper
	CGO_ENABLED=0 $(GO) build -o bin/git-viewer ./cmd/git-viewer
native:
	$(GO) run ./cmd/terminal-lib bin
run:
	$(GO) run ./cmd/git-viewer
smoke: build
	./bin/git-viewer -smoke
dialog-smoke:
	CGO_ENABLED=0 $(GO) build -o bin/native-dialog-probe ./cmd/native-dialog-probe
	./bin/native-dialog-probe
package: helper
	$(GO) tool mygo build -platform darwin/amd64,darwin/arm64 -skip-dmg





