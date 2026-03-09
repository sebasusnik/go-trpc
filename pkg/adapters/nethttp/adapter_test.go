package nethttp_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestServerStartAndShutdown(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":0"})

	// Use a listener to get the actual port
	// Since :0 picks a random port, we need to start and query
	// Use a different approach: start on a known port
	srv2 := nethttp.NewServer(r, nethttp.Config{Addr: "127.0.0.1:18923"})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv2.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Make a request
	resp, err := http.Get("http://127.0.0.1:18923/trpc/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "pong") {
		t.Errorf("expected 'pong' in response, got: %s", body)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv2.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Server should have returned ErrServerClosed
	if err := <-errCh; err != http.ErrServerClosed {
		t.Errorf("expected ErrServerClosed, got %v", err)
	}

	_ = srv // just to verify NewServer with :0 compiles
}

func TestServerDefaultConfig(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	srv := nethttp.NewServer(r, nethttp.Config{})

	if srv.Addr() != ":8080" {
		t.Errorf("expected default addr ':8080', got %q", srv.Addr())
	}
}
