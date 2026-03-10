# Request Cancellation

go-trpc propagates cancellation through Go's `context.Context`. When a client disconnects or aborts a request, the handler's context is cancelled automatically.

## How It Works

```
Client aborts → HTTP connection closes → r.Context().Done() fires → handler exits
```

All procedure handlers receive a `context.Context` as the first argument. When the client disconnects, `ctx.Done()` is closed and `ctx.Err()` returns a non-nil error.

## Queries and Mutations

The request context is cancelled when the client aborts the HTTP request:

```go
router.Query(r, "slowSearch", func(ctx context.Context, input SearchInput) ([]Result, error) {
    results := make([]Result, 0)
    for _, item := range allItems {
        // Check if client is still waiting
        if ctx.Err() != nil {
            return results, ctx.Err()
        }
        if matches(item, input.Query) {
            results = append(results, item)
        }
    }
    return results, nil
})
```

### Client-Side (TypeScript)

Use `AbortController` to cancel requests:

```typescript
const controller = new AbortController();

// Cancel after 5 seconds
setTimeout(() => controller.abort(), 5000);

const result = await trpc.slowSearch.query(
  { query: "test" },
  { signal: controller.signal },
);
```

## SSE Subscriptions

When the client closes the `EventSource`, the SSE connection drops and the subscription's context is cancelled. The channel-producing goroutine should select on `ctx.Done()`:

```go
router.Subscription(r, "events", func(ctx context.Context, input struct{}) (<-chan Event, error) {
    ch := make(chan Event)
    go func() {
        defer close(ch)
        for {
            event, err := waitForEvent()
            if err != nil {
                return
            }
            select {
            case ch <- event:
            case <-ctx.Done():
                return // client disconnected
            }
        }
    }()
    return ch, nil
})
```

Client-side:

```typescript
const eventSource = new EventSource("/trpc/events");

// To cancel:
eventSource.close();
```

## WebSocket Subscriptions

There are two ways a WebSocket subscription can be cancelled:

1. **`subscription.stop` message** — cancels a specific subscription while keeping the connection open
2. **Connection close** — cancels all active subscriptions on that connection

```go
// Same handler pattern — context is cancelled either way
router.Subscription(r, "chat.onMessage", func(ctx context.Context, input RoomInput) (<-chan Message, error) {
    ch := make(chan Message)
    go func() {
        defer close(ch)
        for {
            select {
            case msg := <-messagesBus:
                if msg.RoomID == input.RoomID {
                    select {
                    case ch <- msg:
                    case <-ctx.Done():
                        return
                    }
                }
            case <-ctx.Done():
                return
            }
        }
    }()
    return ch, nil
})
```

Client-side (with `@trpc/client` wsLink):

```typescript
// Subscribe
const subscription = trpc.chat.onMessage.subscribe(
  { roomId: "abc" },
  {
    onData: (message) => console.log(message),
  },
);

// To cancel a specific subscription:
subscription.unsubscribe();

// Or close the entire WebSocket connection by destroying the client
```

## Middleware Awareness

If you write long-running middleware, check `ctx.Err()`:

```go
func SlowValidation() router.Middleware {
    return func(next router.Handler) router.Handler {
        return func(ctx context.Context, req router.Request) (interface{}, error) {
            if ctx.Err() != nil {
                return nil, ctx.Err()
            }
            // ... expensive validation ...
            return next(ctx, req)
        }
    }
}
```

The built-in `Timeout` middleware uses `context.WithTimeout` to automatically cancel handlers that exceed a duration:

```go
r.Use(router.Timeout(5 * time.Second))
```
