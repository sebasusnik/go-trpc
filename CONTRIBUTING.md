# Contributing to go-trpc

## Quick Start

```bash
git clone https://github.com/sebasusnik/go-trpc.git
cd go-trpc
make check   # lint + vet + test (run this before every PR)
```

Requirements: Go 1.25+, [golangci-lint](https://golangci-lint.run/welcome/install/) v2+.

## Development Commands

| Command | Description |
|---|---|
| `make test` | Run all tests |
| `make test-race` | Run tests with race detector |
| `make lint` | Run golangci-lint |
| `make vet` | Run go vet |
| `make fmt` | Format all Go files |
| `make build` | Build the `gotrpc` CLI binary |
| `make coverage` | Generate test coverage report |
| `make check` | **Run before PRs** — lint + vet + test-race |

## Project Structure

```
cmd/gotrpc/          CLI tool (generate, init commands)
pkg/
  router/            Core tRPC router, HTTP handler, middleware, SSE
  codegen/           Go → TypeScript type generation (AST parsing)
  errors/            tRPC error codes and types
  adapters/          Deployment adapters (Lambda, CloudFlare, net/http)
  otel/              OpenTelemetry integration
examples/            Usage examples
```

## Architecture Overview

### Request Flow

```
HTTP Request → ServeHTTP (handler.go)
  → CORS check (handler_cors.go)
  → Route by method (GET=query, POST=mutation)
  → Batch? → handleBatch (handler_batch.go) / handleBatchStream (stream.go)
  → Single? → callProcedure
    → Transformer unwrap input
    → Apply middlewares
    → Handler(ctx, Request) → (output, error)
    → Transformer wrap output
    → JSON response
```

Subscriptions follow a separate path via SSE (`sse.go`): the handler returns a `<-chan T` that gets streamed as Server-Sent Events.

### Codegen Flow

```
gotrpc generate → ParsePackage (type-checked, preferred)
                  ├─ extractFromFile → procedures with full types.Type
                  ├─ extractEnums → Go enum patterns
                  └─ resolvePrefixes → nested router namespaces
               OR ParseDir (AST-only fallback)
                  ├─ extractFromFileAST → procedures with type name strings
                  ├─ extractEnumsFromAST → enum patterns from const blocks
                  └─ extractStructDefsFromAST → struct field info

  → TypeMapper (mapper.go)
    Go types → TypeScript: string→string, []T→T[], *T→T|null, map[K]V→Record<K,V>

  → Generate (generator.go)
    → .d.ts output with AppRouter interface and procedure type map
```

The codegen uses **two parsing strategies**: `ParsePackage` uses `golang.org/x/tools/go/packages` for full type resolution (preferred), while `ParseDir` uses `go/parser` as a fallback when type checking fails. Both extract procedure registrations by scanning for `gotrpc.Query()`, `gotrpc.Mutation()`, and `gotrpc.Subscription()` calls in the AST.

### Middleware Architecture

Middlewares are function decorators applied in registration order:

```go
r.Use(RequestID())          // 1st: adds X-Request-ID
r.Use(BearerAuth(validate)) // 2nd: validates token, enriches context
r.Use(RateLimit(100))       // 3rd: rate limiting
```

Each middleware wraps `next Handler` and can: reject requests (return error), enrich context (add values), or modify the request before passing to `next`.

For subscriptions, middlewares run as a "gate" before the subscription handler — they can reject but don't wrap the channel output.

## Guidelines

### Code Style

- Run `make fmt` before committing
- `make lint` must pass with 0 issues
- Avoid `interface{}` when generics work — use `[I any, O any]`
- Keep the core `pkg/router` dependency-free (stdlib only)

### Error Handling

- Use `trpcerrors.New(code, msg)` for user-facing errors
- Use `trpcerrors.Wrap(cause, code, msg)` to preserve debugging context
- Error codes follow the tRPC protocol (JSON-RPC 2.0 compatible)

### Testing

- All changes need tests — aim for the existing coverage level
- Test both success and error paths
- For codegen changes, test both `ParsePackage` and `ParseDir` paths
- Use `-race` flag: `make test-race`

### Adding a New Middleware

1. Add the function to `pkg/router/middlewares.go`
2. Follow the `func(next Handler) Handler` signature
3. Use context helpers (`GetHeader`, `GetBearerToken`, etc.) to read request data
4. Add tests in `pkg/router/middlewares_test.go`

### Adding a New Adapter

1. Create `pkg/adapters/youradapter/`
2. Implement a function that takes `*router.Router` and adapts it to the target platform
3. Add tests
4. Add an example in `examples/`

## Pull Requests

1. Fork and create a feature branch
2. Run `make check` — must pass
3. Keep commits focused (one logical change per commit)
4. PR title should be concise (under 70 chars)
5. Describe **what** and **why** in the PR description
