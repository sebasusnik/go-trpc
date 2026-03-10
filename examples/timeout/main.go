package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

func main() {
	r := gotrpc.NewRouter()

	// Global timeout: all procedures must complete within 5 seconds.
	r.Use(gotrpc.Timeout(5 * time.Second))

	// A fast query — completes well within the timeout.
	// Try: curl "http://localhost:8080/trpc/fast"
	gotrpc.Query(r, "fast",
		func(ctx context.Context, input struct{}) (string, error) {
			return "done", nil
		},
	)

	// A slow query — simulates a long-running operation.
	// The timeout middleware cancels it after 5 seconds.
	// Try: curl "http://localhost:8080/trpc/slow"
	gotrpc.Query(r, "slow",
		func(ctx context.Context, input struct{}) (string, error) {
			select {
			case <-time.After(30 * time.Second):
				return "finally done", nil
			case <-ctx.Done():
				// Context was cancelled by the Timeout middleware.
				return "", ctx.Err()
			}
		},
	)

	// A query with a tighter per-procedure timeout via procedure-level middleware.
	// This overrides the global 5s with an even stricter 500ms limit.
	// Try: curl "http://localhost:8080/trpc/critical"
	gotrpc.Query(r, "critical",
		func(ctx context.Context, input struct{}) (string, error) {
			select {
			case <-time.After(2 * time.Second):
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
		gotrpc.WithMiddleware(gotrpc.Timeout(500*time.Millisecond)),
	)

	r.PrintRoutes("/trpc")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
