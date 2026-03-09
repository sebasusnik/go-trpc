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

func TestRequestIDMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.RequestID())
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		id, _ := ctx.Value(router.RequestIDKey).(string)
		return id, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Check X-Request-ID header is set
	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header to be set")
	}

	// Check the handler received the ID in context
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Result struct {
			Data string `json:"data"`
		} `json:"result"`
	}
	json.Unmarshal(body, &result)
	if result.Result.Data != reqID {
		t.Errorf("expected handler to receive request ID %q, got %q", reqID, result.Result.Data)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var logged []string
	logger := router.LoggerFunc(func(msg string, args ...any) {
		logged = append(logged, fmt.Sprintf(msg, args...))
	})

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.LoggingMiddleware(logger))
	router.Query(r, "hello", func(ctx context.Context, input struct{}) (string, error) {
		return "world", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/hello")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(logged) == 0 {
		t.Fatal("expected logging middleware to produce log entries")
	}
	found := false
	for _, entry := range logged {
		if strings.Contains(entry, "hello") && strings.Contains(entry, "ok") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected log entry with procedure name and 'ok', got: %v", logged)
	}
}

func TestBearerAuthMiddleware_Valid(t *testing.T) {
	type userKey struct{}

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		if token == "valid-token" {
			return context.WithValue(ctx, userKey{}, "user-123"), nil
		}
		return ctx, fmt.Errorf("invalid token")
	}))
	router.Query(r, "me", func(ctx context.Context, input struct{}) (string, error) {
		user, _ := ctx.Value(userKey{}).(string)
		return user, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/me", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "user-123") {
		t.Errorf("expected user-123 in response, got: %s", body)
	}
}

func TestBearerAuthMiddleware_Missing(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		return ctx, nil
	}))
	router.Query(r, "me", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBearerAuthMiddleware_Invalid(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		return ctx, fmt.Errorf("bad token")
	}))
	router.Query(r, "me", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/me", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIKeyAuthMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.APIKeyAuth("X-API-Key", func(ctx context.Context, key string) (context.Context, error) {
		if key == "secret-key" {
			return ctx, nil
		}
		return ctx, fmt.Errorf("invalid key")
	}))
	router.Query(r, "data", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// With valid key
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/data", nil)
	req.Header.Set("X-API-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with valid key, got %d", resp.StatusCode)
	}

	// Without key
	resp2, err := http.Get(srv.URL + "/trpc/data")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without key, got %d", resp2.StatusCode)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.RateLimit(2)) // 2 requests per second
	router.Query(r, "limited", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Fire 10 requests rapidly — some should be rate limited
	limited := 0
	for i := 0; i < 10; i++ {
		resp, err := http.Get(srv.URL + "/trpc/limited")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			limited++
		}
		resp.Body.Close()
	}

	if limited == 0 {
		t.Error("expected some requests to be rate limited")
	}
}

func TestLoggingMiddlewareErrorPath(t *testing.T) {
	var logged []string
	logger := router.LoggerFunc(func(msg string, args ...any) {
		logged = append(logged, fmt.Sprintf(msg, args...))
	})

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.LoggingMiddleware(logger))
	router.Query(r, "fail", func(ctx context.Context, input struct{}) (string, error) {
		return "", fmt.Errorf("something went wrong")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/fail")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	found := false
	for _, entry := range logged {
		if strings.Contains(entry, "fail") && strings.Contains(entry, "failed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error log entry with 'fail' and 'failed', got: %v", logged)
	}
}

func TestBearerAuthValidatorReturnsTRPCError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		return ctx, errors.New(errors.ErrForbidden, "custom forbidden")
	}))
	router.Query(r, "me", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/me", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Should return 403 (Forbidden), not 401 (Unauthorized)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for TRPCError from BearerAuth validator, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "custom forbidden") {
		t.Errorf("expected 'custom forbidden' in response, got: %s", body)
	}
}

func TestAPIKeyAuthValidatorReturnsTRPCError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.APIKeyAuth("X-API-Key", func(ctx context.Context, key string) (context.Context, error) {
		return ctx, errors.New(errors.ErrForbidden, "key forbidden")
	}))
	router.Query(r, "data", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/data", nil)
	req.Header.Set("X-API-Key", "some-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for TRPCError from APIKeyAuth validator, got %d", resp.StatusCode)
	}
}

func TestRateLimitUnderLimit(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.RateLimit(100)) // high limit
	router.Query(r, "ok", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// 5 requests should all succeed under a 100/s limit
	for i := 0; i < 5; i++ {
		resp, err := http.Get(srv.URL + "/trpc/ok")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestGetProcedureName(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "myProc", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetProcedureName(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/myProc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "myProc") {
		t.Errorf("expected procedure name 'myProc' in response, got: %s", body)
	}
}
