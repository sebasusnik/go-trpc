// Package router implements a tRPC-compatible RPC server for Go.
//
// It provides type-safe procedure registration using Go generics,
// middleware support, Server-Sent Events for subscriptions, batch
// request handling, and CORS configuration.
//
// # Basic Usage
//
//	r := router.NewRouter()
//	router.Query(r, "getUser", func(ctx context.Context, input GetUserInput) (User, error) {
//	    return db.FindUser(input.ID)
//	})
//	router.Mutation(r, "createUser", func(ctx context.Context, input CreateUserInput) (User, error) {
//	    return db.CreateUser(input)
//	})
//	http.ListenAndServe(":8080", r.Handler())
//
// # Middleware
//
// Middlewares wrap procedure handlers and can reject requests, enrich
// context, or add logging/metrics:
//
//	r.Use(router.BearerAuth(validateToken))
//	r.Use(router.RateLimit(100))
//
// # Subscriptions
//
// Subscriptions return a channel that is streamed as Server-Sent Events:
//
//	router.Subscription(r, "events", func(ctx context.Context, input struct{}) (<-chan Event, error) {
//	    ch := make(chan Event)
//	    go produceEvents(ctx, ch)
//	    return ch, nil
//	})
//
// # Nested Routers
//
// Routers can be merged under namespace prefixes:
//
//	appRouter.Merge("user", userRouter) // user.get, user.create, etc.
package router
