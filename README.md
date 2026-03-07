# go-trpc

tRPC Protocol Adapter for Go. Write handlers in Go with typed structs, generate TypeScript types, and consume them with `@trpc/client` — full type-safety, zero Protobuf.

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
import { createTRPCProxyClient, httpBatchLink } from '@trpc/client';
import type { AppRouter } from './generated/router';

const trpc = createTRPCProxyClient<AppRouter>({
  links: [httpBatchLink({ url: 'http://localhost:8080/trpc' })],
});

const user = await trpc.getUser.query({ id: '1' }); // fully typed
```

## Features

- **tRPC protocol compatible** — works with `@trpc/client` v10/v11 unmodified
- **Go generics** — compile-time type safety for handlers
- **Codegen CLI** — AST-based TypeScript generation from Go structs
- **Nested routers** — `router.Merge("user", userRouter)` → `user.get`, `user.create`
- **Middlewares** — chainable, context-based
- **CORS** — built-in configurable CORS support
- **Batch support** — handles tRPC batch queries and mutations
- **Lambda adapter** — deploy on AWS Lambda via SST/Serverless
- **Zero runtime dependencies** — only stdlib for the core runtime

## AWS Lambda / SST

```go
import trpclambda "github.com/sebasusnik/go-trpc/pkg/adapters/lambda"

func main() {
    r := gotrpc.NewRouter()
    // ... register procedures ...
    trpclambda.Start(r)
}
```

## License

MIT
