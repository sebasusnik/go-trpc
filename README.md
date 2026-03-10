# go-trpc

tRPC protocol adapter for Go — write handlers in Go with typed structs, generate TypeScript types, and consume them with `@trpc/client`. Full type-safety, zero Protobuf.

**[Live Demo](https://go-trpc-production.up.railway.app/)** — real-time chat app with SSE subscriptions, built with go-trpc

## Features

- **tRPC protocol compatible** — works with `@trpc/client` v10/v11 unmodified
- **Go generics** — compile-time type safety for handlers
- **Subscriptions** — real-time streaming via Server-Sent Events (SSE)
- **Codegen CLI** — AST-based TypeScript generation from Go structs
- **Nested routers** — `router.Merge("user", userRouter)` → `user.get`, `user.create`
- **Middlewares** — rate limiting, auth, input validation, OpenTelemetry
- **Batch support** — handles tRPC batch queries and mutations
- **Deploy anywhere** — stdlib HTTP, AWS Lambda, Cloudflare Workers
- **Zero runtime dependencies** — only stdlib for the core router

## Quick Start

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

## Generate TypeScript Types

```bash
go install github.com/sebasusnik/go-trpc/cmd/gotrpc@latest
gotrpc generate . --output ./frontend/src/generated/router.d.ts
```

## Use in Frontend

```typescript
import { createTRPCClient, httpLink } from '@trpc/client';
import type { AppRouter } from './generated/router';

const trpc = createTRPCClient<AppRouter>({
  links: [httpLink({ url: 'http://localhost:8080/trpc' })],
});

const user = await trpc.getUser.query({ id: '1' }); // fully typed
```

## Subscriptions (SSE)

```go
gotrpc.Subscription(r, "onMessage",
    func(ctx context.Context, input RoomInput) (<-chan Message, error) {
        ch := make(chan Message)
        // send events on ch, close when done
        return ch, nil
    },
)
```

```typescript
const es = new EventSource('/trpc/onMessage?input={"roomId":"1"}');
es.addEventListener('data', (e) => {
    const msg = JSON.parse(e.data).result.data;
});
```

## Deploy Adapters

```go
// AWS Lambda
import trpclambda "github.com/sebasusnik/go-trpc/pkg/adapters/lambda"
trpclambda.Start(r)

// Cloudflare Workers
import trpccf "github.com/sebasusnik/go-trpc/pkg/adapters/cloudflare"
trpccf.Serve(r)

// Standard HTTP (Docker, Railway, Fly.io, etc.)
http.ListenAndServe(":8080", r.Handler())
```

## Demo

The [live demo](https://go-trpc-production.up.railway.app/) is a real-time chat app showcasing queries, mutations, and SSE subscriptions. Source code is in [`examples/demo/`](./examples/demo/).

## License

MIT
