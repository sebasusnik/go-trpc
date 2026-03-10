<div align="center">

# go-trpc

**End-to-end typesafe APIs with Go + TypeScript**

Write Go handlers. Generate TypeScript types. Use `@trpc/client`. Zero Protobuf.

[![CI](https://github.com/sebasusnik/go-trpc/actions/workflows/ci.yml/badge.svg)](https://github.com/sebasusnik/go-trpc/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sebasusnik/go-trpc.svg)](https://pkg.go.dev/github.com/sebasusnik/go-trpc)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

<br />

<a href="https://go-trpc-production.up.railway.app/">
<img width="1974" height="692" alt="go-trpc demo" src="./assets/demo-screenshot.png" />
</a>

### [Try the Live Demo](https://go-trpc-production.up.railway.app/)

A real-time chat app with rooms, instant messaging via SSE subscriptions, and built-in dev tools — all powered by go-trpc.

[Source code](./examples/demo/) · [Report Bug](https://github.com/sebasusnik/go-trpc/issues)

</div>

<br />

## Architecture

<p align="center">
<picture>
  <img src="./docs/diagram.png" alt="How go-trpc works" width="800" />
</picture>
</p>

<br />

## How It Works

Define a Go handler with typed structs:

```go
gotrpc.Query(r, "getUser",
    func(ctx context.Context, input GetUserInput) (User, error) {
        return db.FindUser(input.ID)
    },
)
```

Generate TypeScript types from your Go code:

```bash
gotrpc generate ./api --output ./web/src/generated/router.d.ts

# Preview without writing to disk (useful for CI)
gotrpc generate ./api --output ./web/src/generated/router.d.ts --dry-run
```

Call it from the frontend with full type-safety:

```typescript
const user = await trpc.getUser.query({ id: '1' }); // fully typed ✓
```

That's it. Go structs become TypeScript types. No schemas, no Protobuf, no manual type definitions.

## Features

| | Feature | |
|---|---|---|
| **Protocol** | tRPC v10/v11 compatible | Works with `@trpc/client` unmodified |
| **Type Safety** | Go generics + codegen | Compile-time safety on both ends |
| **Real-time** | SSE & WebSocket subscriptions | Stream events with `<-chan T`, `wsLink` compatible |
| **Middlewares** | Rate limiting, auth, timeout | Global and per-procedure, chainable |
| **Batching** | Batch queries & mutations | Built-in tRPC batch support |
| **Deploy** | Anywhere | stdlib HTTP, Lambda, Cloudflare Workers |
| **Dependencies** | Minimal | stdlib + 2 deps (`x/time`, `coder/websocket`) |

## Quick Start

### Install

```bash
go get github.com/sebasusnik/go-trpc
go install github.com/sebasusnik/go-trpc/cmd/gotrpc@latest
```

### Server (Go)

```go
package main

import (
    "context"
    "net/http"
    gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type GetUserInput struct {
    ID string `json:"id"`
}

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    r := gotrpc.NewRouter()

    gotrpc.Query(r, "getUser",
        func(ctx context.Context, input GetUserInput) (User, error) {
            return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
        },
    )

    http.ListenAndServe(":8080", r.Handler())
}
```

### Client (TypeScript)

```typescript
import { createTRPCClient, httpLink } from '@trpc/client';
import type { AppRouter } from './generated/router';

const trpc = createTRPCClient<AppRouter>({
  links: [httpLink({ url: 'http://localhost:8080/trpc' })],
});

const user = await trpc.getUser.query({ id: '1' });
//    ^? { id: string; name: string; email: string }
```

## Real-time Subscriptions

Define a subscription once — clients can consume it via SSE or WebSocket:

```go
gotrpc.Subscription(r, "onMessage",
    func(ctx context.Context, input RoomInput) (<-chan Message, error) {
        ch := store.Subscribe(input.RoomID)
        go func() {
            <-ctx.Done()
            store.Unsubscribe(input.RoomID, ch)
        }()
        return ch, nil
    },
)
```

**SSE** (simple, works everywhere):

```typescript
const es = new EventSource('/trpc/onMessage?input={"roomId":"1"}');
es.addEventListener('data', (e) => {
    const msg = JSON.parse(e.data).result.data; // typed Message
});
```

**WebSocket** (multiplexed, uses tRPC's `wsLink`):

```typescript
import { createTRPCClient, splitLink, httpLink, wsLink } from '@trpc/client';

const trpc = createTRPCClient<AppRouter>({
  links: [
    splitLink({
      condition: (op) => op.type === 'subscription',
      true: wsLink({ url: 'ws://localhost:8080/trpc' }),
      false: httpLink({ url: 'http://localhost:8080/trpc' }),
    }),
  ],
});

// Type-safe subscription with automatic reconnection
trpc.onMessage.subscribe({ roomId: '1' }, {
  onData: (msg) => console.log(msg), // typed Message
});
```

## Nested Routers

Organize procedures into namespaces:

```go
userRouter := gotrpc.NewRouter()
gotrpc.Query(userRouter, "get", getUser)
gotrpc.Mutation(userRouter, "create", createUser)

r := gotrpc.NewRouter()
r.Merge("user", userRouter)  // → user.get, user.create
```

## Middlewares

### Global middlewares

Apply to every procedure in the router:

```go
r.Use(gotrpc.RateLimit(100))              // 100 req/s per IP
r.Use(gotrpc.BearerAuth(validateToken))   // JWT/token auth
r.Use(gotrpc.Timeout(5 * time.Second))    // per-request timeout
r.Use(gotrpc.MaxInputSize(4096))          // limit payload size
```

### Per-procedure middlewares

Apply middlewares to individual procedures with `WithMiddleware`:

```go
// Public — no auth required
gotrpc.Query(r, "listItems", listHandler)

// Requires authentication
gotrpc.Query(r, "getItem", getHandler,
    gotrpc.WithMiddleware(authMiddleware),
)

// Admin-only with stacked middlewares
gotrpc.Mutation(r, "createItem", createHandler,
    gotrpc.WithMiddleware(
        adminAuth,
        gotrpc.MaxInputSize(4096),
        gotrpc.Timeout(2 * time.Second),
    ),
)
```

### Rate limit by key

Rate limit by an arbitrary key (user ID, API key, etc.) instead of IP:

```go
gotrpc.Query(r, "search", searchHandler,
    gotrpc.WithMiddleware(
        gotrpc.RateLimitByKey(10, func(ctx context.Context) string {
            return getUserID(ctx) // rate limit per user
        }),
    ),
)
```

## Deploy Anywhere

```go
// Standard HTTP — Docker, Railway, Fly.io, etc.
http.ListenAndServe(":8080", r.Handler())

// AWS Lambda
trpclambda.Start(r)

// Cloudflare Workers
trpccf.Serve(r)
```

## Documentation

| Guide | Description |
|---|---|
| [React Query Integration](./docs/react-query.md) | Using `@trpc/react-query` with go-trpc |
| [Request Cancellation](./docs/cancellation.md) | Context cancellation patterns for queries, SSE, and WebSocket |
| [Contributing](./CONTRIBUTING.md) | Development setup, architecture overview, and guidelines |
| [Roadmap](./docs/ROADMAP.md) | What's built, what's next |

## License

MIT
