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

func TestPanicRecovery(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "boom", func(ctx context.Context, input struct{}) (string, error) {
		panic("something went wrong")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/boom")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errField, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error field in response")
	}
	data := errField["data"].(map[string]interface{})
	if data["code"] != "INTERNAL_SERVER_ERROR" {
		t.Errorf("expected INTERNAL_SERVER_ERROR, got %v", data["code"])
	}
}

func TestPanicRecoveryInBatch(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ok", func(ctx context.Context, input struct{}) (string, error) {
		return "hello", nil
	})
	router.Query(r, "boom", func(ctx context.Context, input struct{}) (string, error) {
		panic("boom")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/ok,boom?batch=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0]["result"] == nil {
		t.Error("first result should be success")
	}
	if results[1]["error"] == nil {
		t.Error("second result should be error (panic)")
	}
}

func TestPanicRecoveryInStream(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ok", func(ctx context.Context, input struct{}) (string, error) {
		return "hello", nil
	})
	router.Query(r, "boom", func(ctx context.Context, input struct{}) (string, error) {
		panic("boom")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/ok,boom?batch=1", nil)
	req.Header.Set("trpc-batch-mode", "stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var results map[string]map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)

	if results["0"]["result"] == nil {
		t.Error("first result should be success")
	}
	if results["1"]["error"] == nil {
		t.Error("second result should be error (panic)")
	}
}

func TestCustomLogger(t *testing.T) {
	logger := &captureLogger{}
	r := router.NewRouter(router.WithLogger(logger))
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

	if len(logger.infos) == 0 {
		t.Error("expected at least one info log")
	}
	if len(logger.debugs) == 0 {
		t.Error("expected at least one debug log")
	}
}

func TestNopLogger(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "hello", func(ctx context.Context, input struct{}) (string, error) {
		return "world", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with NopLogger, got %d", resp.StatusCode)
	}
}

func TestPanicRecoveryLogsError(t *testing.T) {
	logger := &captureLogger{}
	r := router.NewRouter(router.WithLogger(logger))
	router.Query(r, "boom", func(ctx context.Context, input struct{}) (string, error) {
		panic("test panic")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/boom")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(logger.errors) == 0 {
		t.Fatal("expected panic to be logged as error")
	}
	if !strings.Contains(logger.errors[0], "test panic") {
		t.Errorf("expected error log to contain panic message, got %q", logger.errors[0])
	}
}

func TestErrorCauseLoggedOnWrap(t *testing.T) {
	logger := &captureLogger{}
	r := router.NewRouter(router.WithLogger(logger))
	router.Query(r, "fail", func(ctx context.Context, input struct{}) (string, error) {
		cause := fmt.Errorf("connection refused")
		return "", errors.Wrap(cause, errors.ErrInternalError, "database error")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/fail")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(logger.errors) == 0 {
		t.Fatal("expected error cause to be logged")
	}
	if !strings.Contains(logger.errors[0], "connection refused") {
		t.Errorf("expected log to contain cause, got %q", logger.errors[0])
	}
}

func TestErrorResponseHasNoStack(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "fail", func(ctx context.Context, input struct{}) (string, error) {
		return "", errors.New(errors.ErrBadRequest, "bad input")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/fail")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Should not contain "stack" field in the JSON response
	if strings.Contains(content, `"stack"`) {
		t.Errorf("expected no 'stack' field in error response, got: %s", content)
	}
	// Should still have the standard error fields
	if !strings.Contains(content, `"code"`) {
		t.Error("expected 'code' field in error response")
	}
	if !strings.Contains(content, `"message"`) {
		t.Error("expected 'message' field in error response")
	}
}
