package router_test

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestDefaultLoggerViaRequest(t *testing.T) {
	var buf bytes.Buffer
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0) // remove timestamps for easier matching
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
	}()

	// Create router WITHOUT NopLogger — uses defaultLogger
	r := router.NewRouter()
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/ping")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	output := buf.String()
	if !strings.Contains(output, "[trpc]") {
		t.Errorf("expected [trpc] prefix in default logger output, got: %s", output)
	}
}

func TestLoggerFuncInfo(t *testing.T) {
	var msgs []string
	logger := router.LoggerFunc(func(format string, args ...any) {
		msgs = append(msgs, format)
	})

	logger.Info("test %s", "info")
	if len(msgs) == 0 {
		t.Fatal("expected Info to produce output")
	}
	if !strings.Contains(msgs[0], "[trpc]") {
		t.Errorf("expected [trpc] prefix, got: %s", msgs[0])
	}
}

func TestLoggerFuncDebug(t *testing.T) {
	var msgs []string
	logger := router.LoggerFunc(func(format string, args ...any) {
		msgs = append(msgs, format)
	})

	logger.Debug("test %s", "debug")
	if len(msgs) == 0 {
		t.Fatal("expected Debug to produce output")
	}
	if !strings.Contains(msgs[0], "[trpc]") {
		t.Errorf("expected [trpc] prefix, got: %s", msgs[0])
	}
}

func TestLoggerFuncError(t *testing.T) {
	var msgs []string
	logger := router.LoggerFunc(func(format string, args ...any) {
		msgs = append(msgs, format)
	})

	logger.Error("test %s", "error")
	if len(msgs) == 0 {
		t.Fatal("expected Error to produce output")
	}
	if !strings.Contains(msgs[0], "[trpc] ERROR") {
		t.Errorf("expected '[trpc] ERROR' prefix, got: %s", msgs[0])
	}
}
