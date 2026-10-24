GO ?= go

.PHONY: build test vet fmt clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o bin/siphon ./cmd/siphon

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w cmd internal

clean:
	rm -rf bin dist
