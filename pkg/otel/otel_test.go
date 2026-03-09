package otel_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	gotrpcotel "github.com/sebasusnik/go-trpc/pkg/otel"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestMiddlewareCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(gotrpcotel.Middleware(gotrpcotel.WithTracerProvider(tp)))
	router.Query(r, "getUser", func(ctx context.Context, input struct{}) (string, error) {
		return "user", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/getUser")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	span := spans[0]
	if span.Name != "trpc.getUser" {
		t.Errorf("expected span name 'trpc.getUser', got %q", span.Name)
	}

	// Check attributes
	foundSystem := false
	foundMethod := false
	for _, attr := range span.Attributes {
		if string(attr.Key) == "rpc.system" && attr.Value.AsString() == "trpc" {
			foundSystem = true
		}
		if string(attr.Key) == "rpc.method" && attr.Value.AsString() == "getUser" {
			foundMethod = true
		}
	}
	if !foundSystem {
		t.Error("expected rpc.system=trpc attribute")
	}
	if !foundMethod {
		t.Error("expected rpc.method=getUser attribute")
	}
}

func TestMiddlewareRecordsError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(gotrpcotel.Middleware(gotrpcotel.WithTracerProvider(tp)))
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

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	span := spans[0]
	if span.Status.Code.String() != "Error" {
		t.Errorf("expected span status Error, got %s", span.Status.Code.String())
	}
}
