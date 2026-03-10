package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// userKeyType is the context key for the authenticated user.
type userKeyType struct{}

var userKey = userKeyType{}

type ProfileOutput struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
}

func main() {
	r := gotrpc.NewRouter()

	// --- Global middlewares (apply to ALL procedures) ---

	// 1) RequestID — assigns a unique ID to every request (X-Request-ID header).
	r.Use(gotrpc.RequestID())

	// 2) Logging — logs procedure name, duration, and success/failure.
	r.Use(gotrpc.LoggingMiddleware(gotrpc.LoggerFunc(log.Printf)))

	// 3) Timeout — cancel any procedure that takes longer than 10 seconds.
	r.Use(gotrpc.Timeout(10 * time.Second))

	// 4) RateLimit per IP — 10 requests/sec per client IP.
	r.Use(gotrpc.RateLimitByKey(10, func(ctx context.Context) string {
		return gotrpc.GetClientIP(ctx)
	}))

	// --- Public procedure (no auth required) ---

	// Try: curl "http://localhost:8080/trpc/health"
	gotrpc.Query(r, "health",
		func(ctx context.Context, input struct{}) (string, error) {
			return "ok", nil
		},
	)

	// --- Protected procedure (auth via procedure-level middleware) ---

	// Only this procedure requires a Bearer token. The global middlewares
	// (RequestID, Logging, Timeout, RateLimit) still apply.
	//
	// Try: curl -H "Authorization: Bearer secret-token" \
	//   "http://localhost:8080/trpc/getProfile"
	gotrpc.Query(r, "getProfile",
		func(ctx context.Context, input struct{}) (ProfileOutput, error) {
			userID, _ := ctx.Value(userKey).(string)
			return ProfileOutput{
				UserID: userID,
				Name:   "John Doe",
				IP:     gotrpc.GetClientIP(ctx),
			}, nil
		},
		// Procedure-level middleware: BearerAuth only on this route.
		gotrpc.WithMiddleware(gotrpc.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
			if token != "secret-token" {
				return ctx, gotrpc.NewError(gotrpc.ErrUnauthorized, "invalid token")
			}
			ctx = context.WithValue(ctx, userKey, "user-123")
			return ctx, nil
		})),
	)

	// --- Admin mutation with multiple procedure-level middlewares ---

	// Try: curl -X POST -H "Content-Type: application/json" \
	//   -H "Authorization: Bearer admin-token" \
	//   -d '{"action":"reset"}' \
	//   "http://localhost:8080/trpc/adminAction"
	gotrpc.Mutation(r, "adminAction",
		func(ctx context.Context, input struct {
			Action string `json:"action"`
		}) (string, error) {
			return fmt.Sprintf("executed: %s", input.Action), nil
		},
		gotrpc.WithMiddleware(
			gotrpc.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
				if token != "admin-token" {
					return ctx, gotrpc.NewError(gotrpc.ErrForbidden, "admin access required")
				}
				return ctx, nil
			}),
			gotrpc.MaxInputSize(1024), // limit input to 1KB for this procedure
		),
	)

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	r.PrintRoutes("/trpc")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
