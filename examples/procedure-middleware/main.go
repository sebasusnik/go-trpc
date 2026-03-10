package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type userKeyType struct{}

var userKey = userKeyType{}

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	r := gotrpc.NewRouter()

	// Global middlewares — apply to every procedure.
	r.Use(gotrpc.RequestID())
	r.Use(gotrpc.LoggingMiddleware(gotrpc.LoggerFunc(log.Printf)))

	// --- Public procedures (no auth) ---

	// Try: curl "http://localhost:8080/trpc/listItems"
	gotrpc.Query(r, "listItems",
		func(ctx context.Context, input struct{}) ([]Item, error) {
			return []Item{
				{ID: "1", Name: "Widget"},
				{ID: "2", Name: "Gadget"},
			}, nil
		},
	)

	// --- User-level procedures (requires Bearer token) ---

	authMiddleware := gotrpc.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "user-token" || token == "admin-token" {
			ctx = context.WithValue(ctx, userKey, token)
			return ctx, nil
		}
		return ctx, gotrpc.NewError(gotrpc.ErrUnauthorized, "invalid token")
	})

	// Try: curl -H "Authorization: Bearer user-token" \
	//   "http://localhost:8080/trpc/getItem?input=%7B%22id%22:%221%22%7D"
	gotrpc.Query(r, "getItem",
		func(ctx context.Context, input struct {
			ID string `json:"id"`
		}) (Item, error) {
			return Item{ID: input.ID, Name: "Widget"}, nil
		},
		gotrpc.WithMiddleware(authMiddleware),
	)

	// --- Admin-only mutation (auth + rate limit + input size) ---

	adminAuth := gotrpc.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token != "admin-token" {
			return ctx, gotrpc.NewError(gotrpc.ErrForbidden, "admin access required")
		}
		return ctx, nil
	})

	// Try: curl -X POST -H "Content-Type: application/json" \
	//   -H "Authorization: Bearer admin-token" \
	//   -d '{"name":"New Item"}' \
	//   "http://localhost:8080/trpc/createItem"
	gotrpc.Mutation(r, "createItem",
		func(ctx context.Context, input struct {
			Name string `json:"name"`
		}) (Item, error) {
			return Item{ID: "new-id", Name: input.Name}, nil
		},
		gotrpc.WithMiddleware(
			adminAuth,
			gotrpc.MaxInputSize(4096),
			gotrpc.Timeout(2*time.Second),
		),
	)

	// --- Protected subscription ---

	// Try: curl -H "Authorization: Bearer user-token" \
	//   "http://localhost:8080/trpc/feed"
	gotrpc.Subscription(r, "feed",
		func(ctx context.Context, input struct{}) (<-chan string, error) {
			ch := make(chan string)
			go func() {
				defer close(ch)
				for i := 1; i <= 5; i++ {
					select {
					case <-ctx.Done():
						return
					case ch <- fmt.Sprintf("event %d", i):
						time.Sleep(time.Second)
					}
				}
			}()
			return ch, nil
		},
		gotrpc.WithMiddleware(authMiddleware),
	)

	// --- Nested router with inherited procedure middleware ---

	admin := gotrpc.NewRouter()
	gotrpc.Query(admin, "stats",
		func(ctx context.Context, input struct{}) (map[string]int, error) {
			return map[string]int{"users": 42, "items": 100}, nil
		},
		gotrpc.WithMiddleware(adminAuth),
	)
	r.Merge("admin", admin)

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"*"},
	})

	r.PrintRoutes("/trpc")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
