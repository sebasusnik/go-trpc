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

func TestInputValidationSuccess(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "greet", func(ctx context.Context, input ValidatedInput) (string, error) {
		return "Hello, " + input.Name, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/greet?input={"name":"World"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["result"].(map[string]interface{})["data"]
	if data != "Hello, World" {
		t.Errorf("expected 'Hello, World', got %v", data)
	}
}

func TestInputValidationFailure(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "greet", func(ctx context.Context, input ValidatedInput) (string, error) {
		return "Hello, " + input.Name, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/greet?input={"name":""}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errField := result["error"].(map[string]interface{})
	if errField["message"] != "name is required" {
		t.Errorf("expected 'name is required', got %v", errField["message"])
	}
}

func TestInputValidationCustomError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "action", func(ctx context.Context, input ValidatedInputCustomError) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/action", "application/json", strings.NewReader(`{"token":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errField := result["error"].(map[string]interface{})
	data := errField["data"].(map[string]interface{})
	if data["code"] != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED, got %v", data["code"])
	}
}

func TestInputValidationMutation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "create", func(ctx context.Context, input ValidatedInput) (string, error) {
		return "created " + input.Name, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Should fail validation
	resp, err := http.Post(srv.URL+"/trpc/create", "application/json", strings.NewReader(`{"name":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestValidatorReturnsTRPCError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "validated", func(ctx context.Context, input TRPCValidatedInput) (string, error) {
		return input.Value, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/validated?input={"value":"trpc-error"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for TRPCError from validator, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errField := result["error"].(map[string]interface{})
	data := errField["data"].(map[string]interface{})
	if data["code"] != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN code, got %v", data["code"])
	}
}

func TestMutationValidatorReturnsTRPCError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "validated", func(ctx context.Context, input TRPCValidatedInput) (string, error) {
		return input.Value, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/validated", "application/json", strings.NewReader(`{"value":"trpc-error"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for TRPCError from mutation validator, got %d", resp.StatusCode)
	}
}
