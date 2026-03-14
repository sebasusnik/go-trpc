package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type TickInput struct {
	IntervalMs int `json:"intervalMs"`
}

type TickEvent struct {
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	r := gotrpc.NewRouter()

	// Subscription: streams Server-Sent Events to the client.
	// Connect via EventSource: new EventSource("/trpc/tick?input={\"intervalMs\":1000}")
	gotrpc.Subscription(r, "tick",
		func(ctx context.Context, input TickInput) (<-chan TickEvent, error) {
			interval := time.Duration(input.IntervalMs) * time.Millisecond
			if interval <= 0 {
				interval = time.Second
			}

			ch := make(chan TickEvent)
			go func() {
				defer close(ch)
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				seq := 0
				for {
					select {
					case <-ctx.Done():
						// Client disconnected — clean up resources.
						return
					case t := <-ticker.C:
						seq++
						ch <- TickEvent{Seq: seq, Timestamp: t}
					}
				}
			}()
			return ch, nil
		},
	)

	// TrackedEvent example: subscription with custom event IDs.
	// Clients can reconnect with Last-Event-ID header to resume
	// from where they left off.
	gotrpc.Subscription(r, "messages",
		func(ctx context.Context, input struct{}) (<-chan gotrpc.TrackedEvent, error) {
			ch := make(chan gotrpc.TrackedEvent)
			go func() {
				defer close(ch)
				msgs := []struct{ id, text string }{
					{"msg-1", "Hello"},
					{"msg-2", "World"},
					{"msg-3", "Done"},
				}

				lastID := gotrpc.GetLastEventID(ctx)
				start := 0
				for i, m := range msgs {
					if m.id == lastID {
						start = i + 1
						break
					}
				}

				for _, m := range msgs[start:] {
					select {
					case <-ctx.Done():
						return
					case ch <- gotrpc.TrackedEvent{ID: m.id, Data: m.text}:
						time.Sleep(500 * time.Millisecond)
					}
				}
			}()
			return ch, nil
		},
	)

	r.PrintRoutes("/trpc", ":8080")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
