# go-trpc Roadmap

## What we have today

- **Router** — Query/Mutation/Subscription registration, nested routers via Merge, middleware, CORS
- **Codegen** — Parses Go AST, generates `.d.ts` compatible with `@trpc/client` v11
- **CLI** — `gotrpc generate` with watch mode, `gotrpc init` for config scaffolding
- **Lambda adapter** — Deploy to AWS Lambda with Function URLs or API Gateway v2
- **Batch support** — `?batch=1` for multiple procedures in one request
- **Playground** — Go-to-TypeScript type converter (`codegen.ConvertGoToTS`)
- **Wire protocol** — Standard tRPC v11 format, works with official `@trpc/client`
- **Panic recovery** — Catch panics in handlers, return clean 500 INTERNAL_SERVER_ERROR
- **Configurable logging** — `Logger` interface with `WithLogger` option, `NopLogger` to disable
- **Input validation** — Optional `Validate()` interface called automatically before handlers
- **Subscriptions (SSE)** — Server-Sent Events support via `router.Subscription` with tracked events (`TrackedEvent`, `GetLastEventID`) for auto-reconnect
- **Context helpers** — `GetClientIP`, `GetBearerToken`, `GetCookie`, `GetQueryParam`, `GetLastEventID`
- **Response headers** — `SetHeader`, `AddHeader`, `SetCookie` from within procedure handlers via context
- **Error cause chain** — `errors.Wrap`/`Wrapf` with `Unwrap()` for `errors.Is`/`errors.As`
- **Codegen: enum support** — Map Go `const` blocks (string literals and `iota`) to TypeScript union types
- **Content-Type validation** — POST requests require `application/json` Content-Type (415 otherwise)
- **HEAD requests** — Support HEAD method for health checks by proxies/load balancers
- **Error shape compatibility** — `data.stack` field in error responses (`omitempty`, always null in production)
- **Data transformers** — Pluggable transformer system with superjson support
- **httpBatchStreamLink** — Streaming batch responses with `trpc-batch-mode: stream`
- **HTTP status codes** — Correct per-error HTTP status propagation in responses
- **207 Multi-Status** — Batch responses with mixed success/error return 207
- **Middleware library** — Built-in middlewares: `RequestID`, `LoggingMiddleware`, `BearerAuth`, `APIKeyAuth`, `RateLimit`
- **OpenTelemetry** — Optional `pkg/otel` package with tracing spans and `rpc.server.duration` metrics per procedure
- **Adapter: CloudFlare Workers** — Deploy to Workers via `syumai/workers` (`pkg/adapters/cloudflare`)
- **Adapter: standard `net/http`** — Production server with timeouts and graceful shutdown (`pkg/adapters/nethttp`)
- **Codegen: `.ts` runtime output** — Runtime procedure metadata export alongside `.d.ts` types

## Next steps

### v0.2 — Stability + Bug fixes ✅

- [x] **FIX: HTTP status codes** — Propagate error HTTP status in individual responses
- [x] **FIX: 207 Multi-Status** — Return 207 for batch requests with mixed success/error results
- [x] **Panic recovery** — Catch panics in handlers, return clean 500 INTERNAL_SERVER_ERROR instead of crashing the process
- [x] **Request logging** — Make the current `log.Printf` debug logs optional via a configurable logger interface
- [x] **Input validation** — Optional `Validate()` interface that input structs can implement, called automatically before the handler
- [x] **Test coverage** — Unit tests for router (single + batch), handler wire format, codegen output, error mapping
- [x] **Content-Type validation** — Reject POST requests without `application/json` Content-Type (415 Unsupported Media Type)

### v0.3 — Developer experience ✅

- [x] **Subscriptions (SSE)** — Server-Sent Events support for real-time procedures (`router.Subscription`) with tracked events and `Last-Event-ID` auto-reconnect
- [x] **Response headers from procedures** — `SetHeader`, `AddHeader`, `SetCookie` helpers via context
- [x] **Context helpers** — Extract headers, IP, auth token from context without accessing `*http.Request` directly
- [x] **Error cause chain** — `errors.Wrap(err, code, message)` to preserve the original error for logging while returning a clean tRPC error
- [x] **Error `data.stack` field** — Shape-compatible `stack` field in error responses (omitted in production)
- [x] **Codegen: enum support** — Map Go `const` blocks with `iota` to TypeScript union types

### v0.4 — Production readiness ✅

- [x] **Data transformers** — Pluggable transformer system with superjson support (`WithTransformer`)
- [x] **httpBatchStreamLink** — Streaming batch responses via `trpc-batch-mode: stream` header
- [x] **Middleware library** — Built-in middlewares: `RequestID`, `LoggingMiddleware`, `BearerAuth`, `APIKeyAuth`, `RateLimit`
- [x] **OpenTelemetry** — Optional `pkg/otel` package with tracing and `rpc.server.duration` metrics
- [x] **Adapter: CloudFlare Workers** — Adapter for Workers runtime via `syumai/workers`
- [x] **Adapter: standard `net/http`** — Production server with configurable timeouts and graceful shutdown
- [x] **Codegen: multiple output formats** — Support `.ts` (runtime metadata) in addition to `.d.ts` (types only)

### v0.5 — Ecosystem

- [ ] **WebSocket subscriptions** — Support for `wsLink` alongside SSE for real-time procedures
- [ ] **httpBatchLink verification** — Ensure full compatibility with `@trpc/client`'s `httpBatchLink` (partially implemented)
- [ ] **React Query integration docs** — Guide for using `@trpc/react-query` with go-trpc
- [ ] **File uploads** — Support `FormData` / `multipart` input for mutation procedures
- [ ] **Codegen plugins** — Hook system for custom type transformations (e.g. `time.Time` → `string`, custom validators)
- [ ] **Request cancellation docs** — Document and verify `context.Done()` / `AbortController` support
