package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestMiddleware(t *testing.T) {
	r := router.NewRouter()

	// Auth middleware that blocks requests without Authorization header
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			token := router.GetHeader(ctx, "Authorization")
			if token == "" {
				return nil, errors.New(errors.ErrUnauthorized, "missing token")
			}
			return next(ctx, req)
		}
	})

	router.Query(r, "protected",
		func(ctx context.Context, input struct{}) (map[string]string, error) {
			return map[string]string{"status": "ok"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Without auth header
	resp, err := http.Get(srv.URL + "/trpc/protected")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if _, hasError := result["error"]; !hasError {
		t.Fatal("expected error for unauthenticated request")
	}

	// With auth header
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/protected", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var result2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result2)

	if _, hasResult := result2["result"]; !hasResult {
		t.Fatal("expected success for authenticated request")
	}
}

func TestMiddlewaresApplyToSubscriptions(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	// Register a middleware that rejects all requests (simulating auth)
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			return nil, errors.New(errors.ErrUnauthorized, "not authenticated")
		}
	})

	router.Subscription(r, "events",
		func(ctx context.Context, input struct{}) (<-chan string, error) {
			ch := make(chan string, 1)
			ch <- "should not reach here"
			close(ch)
			return ch, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Should get an unauthorized error, NOT an SSE stream
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("expected JSON error response, got: %s", string(body))
	}
	errField, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error field in response, got: %s", string(body))
	}
	if code, _ := errField["code"].(float64); int(code) != errors.ErrUnauthorized {
		t.Errorf("expected error code %d, got %v", errors.ErrUnauthorized, errField["code"])
	}
}

func TestMiddlewaresEnrichSubscriptionContext(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	type ctxKey string
	var captured string

	// Middleware that enriches context
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			ctx = context.WithValue(ctx, ctxKey("user"), "alice")
			return next(ctx, req)
		}
	})

	router.Subscription(r, "events",
		func(ctx context.Context, input struct{}) (<-chan string, error) {
			captured = ctx.Value(ctxKey("user")).(string)
			ch := make(chan string)
			close(ch)
			return ch, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if captured != "alice" {
		t.Errorf("expected middleware to enrich context with user=alice, got %q", captured)
	}
}

func TestSubscriptionMiddlewareRejection(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		return ctx, fmt.Errorf("denied")
	}))
	router.Subscription(r, "events", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// No auth header → middleware should reject before subscription starts
	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 from middleware rejection, got %d: %s", resp.StatusCode, body)
	}
}
