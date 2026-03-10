# Build context: repo root (../../)
# Run: docker build -f examples/demo/Dockerfile -t go-trpc-demo .

# Stage 1: Build Go server
FROM golang:1.24 AS api
WORKDIR /app
COPY go.mod go.sum ./
COPY pkg/ ./pkg/
COPY cmd/ ./cmd/
COPY examples/demo/api/ ./examples/demo/api/
WORKDIR /app/examples/demo/api
RUN go mod tidy && go build -o /server ./main.go

# Stage 2: Build frontend
FROM node:22 AS web
WORKDIR /app
COPY examples/demo/web/package.json examples/demo/web/package-lock.json ./web/
COPY examples/demo/package.json ./
RUN cd web && npm ci
COPY examples/demo/web/ ./web/
RUN cd web && npm run build

# Stage 3: Slim runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=api /server /server
COPY --from=web /app/web/dist /static
EXPOSE 8080
CMD ["/server"]
