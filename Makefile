.PHONY: build test test-race lint vet fmt coverage check clean setup

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/sebasusnik/go-trpc/pkg/router.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o gotrpc ./cmd/gotrpc

test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -w .

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

check: lint vet test-race

setup:
	git config core.hooksPath .githooks

clean:
	rm -f gotrpc coverage.out
