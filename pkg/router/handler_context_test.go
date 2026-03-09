package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestGetClientIP(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ip", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetClientIP(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Test X-Forwarded-For
	req, _ := http.NewRequest("GET", srv.URL+"/trpc/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "203.0.113.1" {
		t.Errorf("expected first XFF IP, got %v", data)
	}
}

func TestGetClientIPXRealIP(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ip", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetClientIP(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/ip", nil)
	req.Header.Set("X-Real-IP", "198.51.100.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "198.51.100.1" {
		t.Errorf("expected X-Real-IP, got %v", data)
	}
}

func TestGetBearerToken(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "token", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetBearerToken(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/token", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "my-secret-token" {
		t.Errorf("expected 'my-secret-token', got %v", data)
	}
}

func TestGetBearerTokenEmpty(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "token", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetBearerToken(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/token")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "" {
		t.Errorf("expected empty string, got %v", data)
	}
}

func TestGetCookie(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "session", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetCookie(ctx, "session_id"), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "abc123"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "abc123" {
		t.Errorf("expected 'abc123', got %v", data)
	}
}

func TestGetQueryParam(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "param", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetQueryParam(ctx, "foo"), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/param?foo=bar")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "bar" {
		t.Errorf("expected 'bar', got %v", data)
	}
}

func TestSetHeader(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "cached", func(ctx context.Context, input struct{}) (string, error) {
		router.SetHeader(ctx, "Cache-Control", "max-age=3600")
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/cached")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Cache-Control"); got != "max-age=3600" {
		t.Errorf("expected Cache-Control 'max-age=3600', got %q", got)
	}
}

func TestAddHeader(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "multi", func(ctx context.Context, input struct{}) (string, error) {
		router.AddHeader(ctx, "X-Custom", "value1")
		router.AddHeader(ctx, "X-Custom", "value2")
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/multi")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	values := resp.Header.Values("X-Custom")
	if len(values) != 2 || values[0] != "value1" || values[1] != "value2" {
		t.Errorf("expected two X-Custom headers, got %v", values)
	}
}

func TestSetCookie(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "login", func(ctx context.Context, input struct{}) (string, error) {
		router.SetCookie(ctx, &http.Cookie{
			Name:  "session",
			Value: "abc123",
			Path:  "/",
		})
		return "logged in", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/login", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	cookies := resp.Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session" && c.Value == "abc123" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'session' cookie with value 'abc123'")
	}
}

func TestCustomBasePath(t *testing.T) {
	r := router.NewRouter(
		router.WithBasePath("/api/rpc"),
		router.WithLogger(router.NopLogger),
	)

	router.Query(r, "ping",
		func(ctx context.Context, input struct{}) (string, error) {
			return "pong", nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Should work with custom base path
	resp, err := http.Get(srv.URL + "/api/rpc/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with custom basePath, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resultField, _ := result["result"].(map[string]interface{})
	if data, _ := resultField["data"].(string); data != "pong" {
		t.Errorf("expected pong, got %v", data)
	}

	// Should NOT work with default /trpc path
	resp2, err := http.Get(srv.URL + "/trpc/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for wrong basePath, got %d", resp2.StatusCode)
	}
}

func TestBasePathNormalization(t *testing.T) {
	tests := []struct {
		input    string
		wantPath string
	}{
		{"api", "/api/ping"},
		{"/api/", "/api/ping"},
		{"/v1/trpc/", "/v1/trpc/ping"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := router.NewRouter(
				router.WithBasePath(tt.input),
				router.WithLogger(router.NopLogger),
			)
			router.Query(r, "ping",
				func(ctx context.Context, input struct{}) (string, error) {
					return "pong", nil
				},
			)

			srv := httptest.NewServer(r.Handler())
			defer srv.Close()

			resp, err := http.Get(srv.URL + tt.wantPath)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("basePath %q: expected 200 at %s, got %d", tt.input, tt.wantPath, resp.StatusCode)
			}
		})
	}
}

func TestGetBearerTokenNonBearerScheme(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "token", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetBearerToken(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/token", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "" {
		t.Errorf("expected empty string for Basic auth scheme, got %v", data)
	}
}

func TestGetClientIPRemoteAddrFallback(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ip", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetClientIP(ctx), nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// No X-Forwarded-For or X-Real-IP — should use RemoteAddr
	resp, err := http.Get(srv.URL + "/trpc/ip")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"].(string)
	if data == "" {
		t.Error("expected non-empty client IP from RemoteAddr fallback")
	}
	// Should be the loopback IP (127.0.0.1 or ::1)
	if !strings.Contains(data, "127.0.0.1") && !strings.Contains(data, "::1") {
		t.Errorf("expected loopback IP, got %q", data)
	}
}

func TestQueryContextCancellation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	started := make(chan struct{})
	router.Query(r, "slow", func(ctx context.Context, input struct{}) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/trpc/slow", nil)

	go func() {
		<-started
		cancel()
	}()

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	// The request should have been cancelled — either err is non-nil (client cancelled)
	// or the server returned an error response. Both are acceptable.
	if err == nil && resp.StatusCode == http.StatusOK {
		t.Error("expected request to be cancelled, but got 200 OK")
	}
}
