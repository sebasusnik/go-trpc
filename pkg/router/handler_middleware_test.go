package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestProcedureLevelMiddleware_Query(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	// Public query — no procedure-level middleware
	router.Query(r, "public", func(ctx context.Context, input struct{}) (string, error) {
		return "public", nil
	})

	// Protected query — requires auth via procedure-level middleware
	router.Query(r, "admin", func(ctx context.Context, input struct{}) (string, error) {
		return "admin-data", nil
	}, router.WithMiddleware(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "admin-token" {
			return ctx, nil
		}
		return ctx, fmt.Errorf("invalid")
	})))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Public should work without auth
	resp, err := http.Get(srv.URL + "/trpc/public")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("public: expected 200, got %d", resp.StatusCode)
	}

	// Admin without auth should fail
	resp2, err := http.Get(srv.URL + "/trpc/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("admin without auth: expected 401, got %d", resp2.StatusCode)
	}

	// Admin with auth should succeed
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/admin", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("admin with auth: expected 200, got %d", resp3.StatusCode)
	}
	body, _ := io.ReadAll(resp3.Body)
	if !strings.Contains(string(body), "admin-data") {
		t.Errorf("expected admin-data in response, got: %s", body)
	}
}

func TestProcedureLevelMiddleware_Mutation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	router.Mutation(r, "protectedCreate", func(ctx context.Context, input struct{}) (string, error) {
		return "created", nil
	}, router.WithMiddleware(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "valid" {
			return ctx, nil
		}
		return ctx, fmt.Errorf("denied")
	})))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Without auth
	resp, err := http.Post(srv.URL+"/trpc/protectedCreate", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// With auth
	req, _ := http.NewRequest("POST", srv.URL+"/trpc/protectedCreate", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestProcedureLevelMiddleware_Subscription(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	router.Subscription(r, "protectedEvents", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string, 1)
		ch <- "event"
		close(ch)
		return ch, nil
	}, router.WithMiddleware(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "sub-token" {
			return ctx, nil
		}
		return ctx, fmt.Errorf("denied")
	})))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Without auth — should be rejected
	resp, err := http.Get(srv.URL + "/trpc/protectedEvents")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// With auth — should succeed and get SSE stream
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/protectedEvents", nil)
	req.Header.Set("Authorization", "Bearer sub-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}
	body, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body), "event") {
		t.Errorf("expected 'event' in SSE stream, got: %s", body)
	}
}

func TestProcedureLevelMiddleware_WithGlobalMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	type ctxKey string
	// Global middleware sets a context value
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			ctx = context.WithValue(ctx, ctxKey("global"), "yes")
			return next(ctx, req)
		}
	})

	// Procedure-level middleware adds another context value
	router.Query(r, "both", func(ctx context.Context, input struct{}) (map[string]string, error) {
		return map[string]string{
			"global": ctx.Value(ctxKey("global")).(string),
			"local":  ctx.Value(ctxKey("local")).(string),
		}, nil
	}, router.WithMiddleware(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			ctx = context.WithValue(ctx, ctxKey("local"), "yes")
			return next(ctx, req)
		}
	}))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/both")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Both middleware values should be present
	if !strings.Contains(string(body), `"global":"yes"`) || !strings.Contains(string(body), `"local":"yes"`) {
		t.Errorf("expected both global and local middleware values, got: %s", body)
	}
}

func TestProcedureLevelMiddleware_ExecutionOrder(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	type ctxKey string

	// Global middleware runs first
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			order := "global"
			ctx = context.WithValue(ctx, ctxKey("order"), order)
			return next(ctx, req)
		}
	})

	// Procedure middleware runs second, appending to the order
	router.Query(r, "ordered", func(ctx context.Context, input struct{}) (string, error) {
		return ctx.Value(ctxKey("order")).(string), nil
	}, router.WithMiddleware(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			prev := ctx.Value(ctxKey("order")).(string)
			ctx = context.WithValue(ctx, ctxKey("order"), prev+",local")
			return next(ctx, req)
		}
	}))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/ordered")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "global,local") {
		t.Errorf("expected execution order 'global,local', got: %s", body)
	}
}

func TestProcedureLevelMiddleware_MergedRouter(t *testing.T) {
	child := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(child, "secret", func(ctx context.Context, input struct{}) (string, error) {
		return "secret-data", nil
	}, router.WithMiddleware(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "child-token" {
			return ctx, nil
		}
		return ctx, fmt.Errorf("denied")
	})))

	parent := router.NewRouter(router.WithLogger(router.NopLogger))
	parent.Merge("child", child)

	srv := httptest.NewServer(parent.Handler())
	defer srv.Close()

	// Without auth
	resp, err := http.Get(srv.URL + "/trpc/child.secret")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// With auth
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/child.secret", nil)
	req.Header.Set("Authorization", "Bearer child-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
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
