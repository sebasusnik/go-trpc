# go-trpc Roadmap

## What we have today

- **Router** — Query/Mutation registration, nested routers via Merge, middleware, CORS
- **Codegen** — Parses Go AST, generates `.d.ts` compatible with `@trpc/client` v11
- **CLI** — `gotrpc generate` with watch mode, `gotrpc init` for config scaffolding
- **Lambda adapter** — Deploy to AWS Lambda with Function URLs or API Gateway v2
- **Batch support** — `?batch=1` for multiple procedures in one request
- **Playground** — Go-to-TypeScript type converter (`codegen.ConvertGoToTS`)
- **Wire protocol** — Standard tRPC v11 format, works with official `@trpc/client`

## Next steps

### v0.2 — Stability

- [ ] **Panic recovery** — Catch panics in handlers, return clean 500 INTERNAL_SERVER_ERROR instead of crashing the process
- [ ] **Request logging** — Make the current `log.Printf` debug logs optional via a configurable logger interface
- [ ] **Input validation** — Optional `Validate()` interface that input structs can implement, called automatically before the handler
- [ ] **Test coverage** — Unit tests for router (single + batch), handler wire format, codegen output, error mapping

### v0.3 — Developer experience

- [ ] **Subscriptions (SSE)** — Server-Sent Events support for real-time procedures (`gotrpc.Subscription`)
- [ ] **Context helpers** — Extract headers, IP, auth token from context without accessing `*http.Request` directly
- [ ] **Error cause chain** — `errors.Wrap(err, code, message)` to preserve the original error for logging while returning a clean tRPC error
- [ ] **Codegen: enum support** — Map Go `const` blocks with `iota` to TypeScript union types

### v0.4 — Production readiness

- [ ] **Middleware library** — Built-in middlewares: rate limiting, request ID, auth (JWT/API key), logging
- [ ] **OpenTelemetry** — Optional tracing/metrics integration for each procedure call
- [ ] **Adapter: CloudFlare Workers** — Adapter for Workers runtime alongside Lambda
- [ ] **Adapter: standard `net/http`** — Explicit adapter for non-Lambda deployments with graceful shutdown
- [ ] **Codegen: multiple output formats** — Support `.ts` (runtime) in addition to `.d.ts` (types only)

### v0.5 — Ecosystem

- [ ] **httpBatchLink support** — Ensure full compatibility with `@trpc/client`'s `httpBatchLink`
- [ ] **React Query integration docs** — Guide for using `@trpc/react-query` with go-trpc
- [ ] **File uploads** — Support `FormData` / `multipart` input for mutation procedures
- [ ] **Codegen plugins** — Hook system for custom type transformations (e.g. `time.Time` → `string`, custom validators)
