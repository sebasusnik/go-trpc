package main

import (
	"context"
	"fmt"
	"log"

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

	// Middlewares execute in registration order.
	// 1) RequestID — assigns a unique ID to every request (X-Request-ID header).
	r.Use(gotrpc.RequestID())

	// 2) Logging — logs procedure name, duration, and success/failure.
	r.Use(gotrpc.LoggingMiddleware(gotrpc.LoggerFunc(log.Printf)))

	// 3) BearerAuth — validates the Authorization header and enriches context.
	//    The validate function receives the raw token and returns a context with
	//    user info, or an error to reject the request.
	r.Use(gotrpc.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		// In production, verify token against your auth provider here.
		if token != "secret-token" {
			return ctx, gotrpc.NewError(gotrpc.ErrUnauthorized, "invalid token")
		}
		// Enrich context with user information for downstream handlers.
		ctx = context.WithValue(ctx, userKey, "user-123")
		return ctx, nil
	}))

	// 4) RateLimit — allows up to 100 requests/sec across all procedures.
	r.Use(gotrpc.RateLimit(100))

	// A protected query that uses context values set by middlewares.
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
	)

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	r.PrintRoutes("/trpc")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
