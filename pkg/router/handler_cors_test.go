package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestCORS(t *testing.T) {
	r := setupRouter()
	r.WithCORS(router.CORSConfig{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Preflight request
	req, _ := http.NewRequest("OPTIONS", srv.URL+"/trpc/getUser", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("expected CORS origin header, got %s", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSWildcardOrigin(t *testing.T) {
	r := setupRouter()
	r.WithCORS(router.CORSConfig{
		AllowedOrigins: []string{"*"},
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("OPTIONS", srv.URL+"/trpc/getUser", nil)
	req.Header.Set("Origin", "http://any-domain.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") != "http://any-domain.com" {
		t.Errorf("expected wildcard to allow any origin, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSNonMatchingOrigin(t *testing.T) {
	r := setupRouter()
	r.WithCORS(router.CORSConfig{
		AllowedOrigins: []string{"http://allowed.com"},
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("OPTIONS", srv.URL+"/trpc/getUser", nil)
	req.Header.Set("Origin", "http://not-allowed.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS headers for non-matching origin, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSAllowedHeadersAndMaxAge(t *testing.T) {
	r := setupRouter()
	r.WithCORS(router.CORSConfig{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Custom"},
		MaxAge:         3600,
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("OPTIONS", srv.URL+"/trpc/getUser", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	allowHeaders := resp.Header.Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "Authorization") || !strings.Contains(allowHeaders, "X-Custom") {
		t.Errorf("expected allowed headers to include Authorization and X-Custom, got %q", allowHeaders)
	}

	maxAge := resp.Header.Get("Access-Control-Max-Age")
	if maxAge != "3600" {
		t.Errorf("expected Max-Age 3600, got %q", maxAge)
	}
}
