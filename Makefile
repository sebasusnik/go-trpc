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

demo: build
	@rm -rf /tmp/myapp && mkdir -p /tmp/myapp
	@mkdir -p /tmp/gotrpc-bin && cp gotrpc /tmp/gotrpc-bin/gotrpc
	@cd /tmp/myapp && go mod init myapp > /dev/null 2>&1 && go mod edit -require github.com/sebasusnik/go-trpc@v0.0.0 -replace github.com/sebasusnik/go-trpc=$(CURDIR)
	@printf "import type { AppRouter } from './generated/router';\n\nconst trpc = createTRPCClient<AppRouter>({ url: '/trpc' });\n\nconst user = await trpc.getUser.query({ id: '1' });\n//    ^? { id: string; name: string; email: string }\n" > /tmp/gotrpc-demo.ts
	vhs demo.tape

clean:
	rm -f gotrpc coverage.out
