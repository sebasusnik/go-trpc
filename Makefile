.PHONY: build test test-race lint vet fmt coverage check clean

build:
	go build -o gotrpc ./cmd/gotrpc

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

clean:
	rm -f gotrpc coverage.out
