.PHONY: dev dev-api dev-web generate watch install build clean docker

# Start Go server + Vite dev server
dev:
	@make -j2 dev-api dev-web

# Run Go server for local development
dev-api:
	cd api && go run ./cmd/local

# Run Vite dev server (proxies /trpc to localhost:8080)
dev-web:
	cd web && npm run dev

# Watch Go files and regenerate TypeScript types on change
watch:
	go run ../../cmd/gotrpc generate $(PWD)/api --output $(PWD)/web/src/generated/router.d.ts --watch

# Generate TypeScript types from Go handlers (one-shot)
generate:
	go run ../../cmd/gotrpc generate $(PWD)/api --output $(PWD)/web/src/generated/router.d.ts

# Install frontend dependencies
install:
	cd web && npm install

# Build for production
build:
	cd api && go build -o ../dist/server ./main.go
	cd web && npm run build

# Build Docker image (from repo root for full context)
docker:
	cd ../.. && docker build -f examples/demo/Dockerfile -t go-trpc-demo .

# Run Docker image locally
docker-run:
	docker run -p 8080:8080 go-trpc-demo

# Clean build artifacts
clean:
	rm -rf dist web/dist
