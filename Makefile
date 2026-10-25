GO ?= go

PREFIX ?= $(HOME)/.local
KDEPLASMA ?= both

.PHONY: build test vet fmt clean install-kde uninstall-kde test-kde

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o bin/siphon ./cmd/siphon

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w cmd internal

install-kde: build
	./kde/install.sh --prefix "$(PREFIX)" --plasma "$(KDEPLASMA)"

uninstall-kde:
	./kde/install.sh --prefix "$(PREFIX)" --plasma "$(KDEPLASMA)" --uninstall

test-kde:
	./kde/test-wrapper.sh

clean:
	rm -rf bin dist
