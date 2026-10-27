GO ?= go
BINARY ?= pldi
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet lint shellcheck tidy release

build:
	$(GO) build -ldflags "-X github.com/punchlistdigital/pldi/cmd.Version=$(VERSION)" -o $(BINARY) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

shellcheck:
	$(GO) test ./internal/snapshot -run TestShellcheck -count=1

tidy:
	$(GO) mod tidy

release:
	goreleaser release --clean --snapshot
