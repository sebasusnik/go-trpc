# React Query Integration

go-trpc works with `@trpc/react-query` for React apps that need caching, refetching, and optimistic updates.

## Setup

```bash
npm install @trpc/react-query @trpc/client @tanstack/react-query
```

Create the typed hooks:

```typescript
// src/trpc.ts
import { createTRPCReact } from "@trpc/react-query";
import { httpLink } from "@trpc/client";
import type { AppRouter } from "./generated/router";

export const trpc = createTRPCReact<AppRouter>();

export const trpcClient = trpc.createClient({
  links: [
    httpLink({
      url: "/trpc",
    }),
  ],
});
```

## Provider

Wrap your app with both providers:

```tsx
// src/main.tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { trpc, trpcClient } from "./trpc";

const queryClient = new QueryClient();

createRoot(document.getElementById("root")!).render(
  <trpc.Provider client={trpcClient} queryClient={queryClient}>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </trpc.Provider>,
);
```

## Queries

```tsx
function UserProfile({ id }: { id: string }) {
  const { data, isLoading, error } = trpc.getUser.useQuery({ id });

  if (isLoading) return <div>Loading...</div>;
  if (error) return <div>Error: {error.message}</div>;
  return <div>{data.name}</div>;
}
```

## Mutations

```tsx
function CreateUser() {
  const utils = trpc.useUtils();
  const mutation = trpc.createUser.useMutation({
    onSuccess: () => {
      // Invalidate related queries
      utils.getUser.invalidate();
    },
  });

  return (
    <button onClick={() => mutation.mutate({ name: "Alice", email: "alice@example.com" })}>
      Create
    </button>
  );
}
```

## Subscriptions (WebSocket)

For real-time subscriptions, add `wsLink` with `splitLink`:

```typescript
import { splitLink, httpLink, wsLink } from "@trpc/client";

export const trpcClient = trpc.createClient({
  links: [
    splitLink({
      condition: (op) => op.type === "subscription",
      true: wsLink({ url: "ws://localhost:8080/trpc" }),
      false: httpLink({ url: "/trpc" }),
    }),
  ],
});
```

Then use the subscription hook:

```tsx
function ChatMessages({ roomId }: { roomId: string }) {
  const [messages, setMessages] = useState<Message[]>([]);

  trpc.chat.onMessage.useSubscription(
    { roomId },
    {
      onData: (message) => {
        setMessages((prev) => [...prev, message]);
      },
    },
  );

  return (
    <ul>
      {messages.map((m) => (
        <li key={m.id}>{m.content}</li>
      ))}
    </ul>
  );
}
```

## When to Use

| Use case | Recommendation |
|----------|---------------|
| Caching, refetching, pagination | `@trpc/react-query` |
| Simple fetch-and-display | Vanilla `@trpc/client` is enough |
| Non-React (Vue, Svelte, etc.) | Vanilla `@trpc/client` |

The vanilla client (`createTRPCClient`) is lighter and sufficient for apps that don't need React Query's cache management. The go-trpc demo uses the vanilla client for this reason.
