# Build context: repo root (../../)
# Run: docker build -f examples/demo/Dockerfile -t go-trpc-demo .

# Stage 1: Build Go server
FROM golang:1.25 AS api
WORKDIR /app
COPY go.mod go.sum ./
COPY pkg/ ./pkg/
COPY cmd/ ./cmd/
COPY examples/demo/api/ ./examples/demo/api/
WORKDIR /app/examples/demo/api
RUN go mod tidy && CGO_ENABLED=0 go build -o /server ./main.go

# Stage 2: Build frontend
FROM node:22 AS web
WORKDIR /app
COPY examples/demo/web/package.json ./web/
COPY examples/demo/package.json examples/demo/package-lock.json ./
RUN cd web && npm install
COPY examples/demo/web/ ./web/
RUN cd web && npm run build

# Stage 3: Alpine runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=api /server /server
COPY --from=web /app/web/dist /static
EXPOSE 8080
CMD ["/server"]
