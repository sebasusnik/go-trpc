package cloudflare_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/adapters/cloudflare"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestHandlerReturnsWorkingHandler(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	handler := cloudflare.Handler(r)

	req := httptest.NewRequest("GET", "/trpc/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "pong") {
		t.Errorf("expected 'pong' in response, got: %s", body)
	}
}
