# go-trpc Demo — Chat Rooms

Interactive demo of [go-trpc](https://github.com/sebasusnik/go-trpc): a Go backend speaking the tRPC protocol with real-time SSE subscriptions, auto-generated TypeScript types, and a React frontend.

## What this demo shows

1. **Go Backend** — Chat handlers using `gotrpc.Query`, `gotrpc.Mutation`, and `gotrpc.Subscription`
2. **Real-time SSE** — Live message streaming via Server-Sent Events
3. **Codegen** — TypeScript types auto-generated from Go source via AST parsing
4. **React Frontend** — Fully-typed tRPC client with autocompletion for every procedure
5. **Request Log** — Live view of every tRPC request/response in the UI
6. **Type Playground** — Convert Go structs to TypeScript in the browser
7. **Protection Middlewares** — Rate limiting, per-IP connection limits, input size limits

## Quick Start

### Prerequisites

- Go 1.24+
- Node.js 22+

### Run locally

```bash
# 1. Install frontend dependencies
cd web && npm install && cd ..

# 2. Start API + Frontend
make dev
```

Open http://localhost:5173 — the frontend proxies `/trpc` requests to the Go backend on `:8080`.

### Using Make

```bash
make install   # Install npm dependencies
make dev       # Start both API and frontend
make generate  # Re-generate TypeScript types from Go
make docker    # Build Docker image
```

## Project Structure

```
go-trpc-demo/
├── api/                    # Go backend
│   ├── main.go             # Production HTTP server (static + API)
│   ├── cmd/local/main.go   # Local dev server with CORS
│   └── app/
│       ├── types.go        # Room, Message, input/output types
│       ├── store.go        # In-memory ChatStore with pub/sub
│       ├── handlers.go     # Chat handlers + SSE subscription
│       └── router.go       # Router setup with middlewares
├── web/                    # React + Vite + TypeScript + Tailwind v4
│   └── src/
│       ├── trpc.ts         # Typed tRPC client with request logging
│       ├── App.tsx         # Layout: rooms + chat + dev tools
│       └── components/     # ChatRoom, RoomList, CodePanel, RequestLog
├── Dockerfile              # Multi-stage build for deployment
└── Makefile
```

## API Procedures

| Procedure          | Type         | Method | Path                       | Description                    |
|--------------------|--------------|--------|----------------------------|--------------------------------|
| `room.list`        | query        | GET    | `/trpc/room.list`          | List all rooms                 |
| `room.create`      | mutation     | POST   | `/trpc/room.create`        | Create a new room              |
| `room.messages`    | query        | GET    | `/trpc/room.messages`      | Get message history for a room |
| `chat.send`        | mutation     | POST   | `/trpc/chat.send`          | Send a message                 |
| `chat.subscribe`   | subscription | GET    | `/trpc/chat.subscribe`     | SSE stream of new messages     |
| `health.check`     | query        | GET    | `/trpc/health.check`       | Health check                   |

## How go-trpc works

### 1. Define handlers in Go

```go
// Subscription — returns a channel for SSE streaming
gotrpc.Subscription(chatRouter, "subscribe",
    func(ctx context.Context, input SubscribeRoomInput) (<-chan Message, error) {
        ch := store.Subscribe(input.RoomID)
        go func() {
            <-ctx.Done()
            store.Unsubscribe(input.RoomID, ch)
        }()
        return ch, nil
    },
)

// Mutation — send a message and broadcast to SSE subscribers
gotrpc.Mutation(chatRouter, "send",
    func(ctx context.Context, input SendMessageInput) (Message, error) {
        msg := store.AddMessage(input)
        store.Broadcast(input.RoomID, msg)
        return msg, nil
    },
)
```

### 2. Generate TypeScript types

```bash
gotrpc generate ./api --output ./web/src/generated/router.d.ts
```

This parses Go source via AST, maps Go types to TypeScript (`string` → `string`, `int` → `number`, `omitempty` → `?`), and generates an `AppRouter` type.

### 3. Use in React with full type-safety

```typescript
import { createTRPCClient, httpLink } from "@trpc/client";
import type { AppRouter } from "./generated/router";

const trpc = createTRPCClient<AppRouter>({
  links: [httpLink({ url: "/trpc" })],
});

const rooms = await trpc.room.list.query();
//    ^? { rooms: Room[] } — fully typed

const msg = await trpc.chat.send.mutate({
  roomId: "room-1",
  username: "alice",
  content: "Hello!",
});
//    ^? Message — autocompletion for all fields

// SSE subscription via EventSource
const es = new EventSource(`/trpc/chat.subscribe?input=${encoded}`);
es.addEventListener("data", (e) => {
  const msg = JSON.parse(e.data).result?.data; // Message
});
```

## Type Mapping

| Go                  | TypeScript              |
|---------------------|-------------------------|
| `string`            | `string`                |
| `int`, `float64`    | `number`                |
| `bool`              | `boolean`               |
| `*T`                | `T \| null`             |
| `[]T`               | `T[]`                   |
| `map[string]V`      | `Record<string, V>`     |
| `time.Time`         | `string` (ISO 8601)     |
| `omitempty` tag      | optional field (`?`)    |
| `json:"-"` tag      | omitted                 |

## Tech Stack

- **Backend**: Go 1.24+ with [go-trpc](https://github.com/sebasusnik/go-trpc)
- **Frontend**: React 19, Vite 6, TypeScript 5.7, Tailwind CSS v4
- **Client**: `@trpc/client` — go-trpc generates types compatible with the official tRPC client
- **Deploy**: Docker on Koyeb (free tier)
