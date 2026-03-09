package router_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestQuerySuccess(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/getUser?input={"id":"1"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	resultField, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatal("expected result field")
	}
	data, ok := resultField["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data field")
	}
	if data["id"] != "1" {
		t.Errorf("expected id=1, got %v", data["id"])
	}
	if data["name"] != "John" {
		t.Errorf("expected name=John, got %v", data["name"])
	}
}

func TestMutationSuccess(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	body := `{"name":"Jane","email":"jane@example.com"}`
	resp, err := http.Post(srv.URL+"/trpc/createUser", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	data := result["result"].(map[string]interface{})["data"].(map[string]interface{})
	if data["name"] != "Jane" {
		t.Errorf("expected name=Jane, got %v", data["name"])
	}
}

func TestQueryNotFound(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/getUser?input={"id":"not-found"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	errField, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error field")
	}
	if errField["message"] != "user not found" {
		t.Errorf("expected 'user not found', got %v", errField["message"])
	}
	if int(errField["code"].(float64)) != errors.ErrNotFound {
		t.Errorf("expected error code %d, got %v", errors.ErrNotFound, errField["code"])
	}
}

func TestProcedureNotFound(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/nonExistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	errField := result["error"].(map[string]interface{})
	if errField["message"] != "procedure not found: nonExistent" {
		t.Errorf("unexpected error message: %v", errField["message"])
	}
}

func TestNestedRouter(t *testing.T) {
	userRouter := router.NewRouter()
	router.Query(userRouter, "get",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John"}, nil
		},
	)

	appRouter := router.NewRouter()
	appRouter.Merge("user", userRouter)

	srv := httptest.NewServer(appRouter.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/user.get?input={"id":"1"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	data := result["result"].(map[string]interface{})["data"].(map[string]interface{})
	if data["id"] != "1" {
		t.Errorf("expected id=1, got %v", data["id"])
	}
}

func TestWrongMethodForProcedure(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Try to POST to a query procedure
	body := `{"id":"1"}`
	resp, err := http.Post(srv.URL+"/trpc/getUser", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if _, hasError := result["error"]; !hasError {
		t.Fatal("expected error when using wrong method")
	}
}

func TestQueryErrorHTTPStatus(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/getUser?input={"id":"not-found"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestMutationErrorHTTPStatus(t *testing.T) {
	r := router.NewRouter()
	router.Mutation(r, "adminAction",
		func(ctx context.Context, input struct{}) (string, error) {
			return "", errors.New(errors.ErrUnauthorized, "unauthorized")
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/adminAction", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestInvalidPath(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/invalid-path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHEADRequest(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "health", func(ctx context.Context, input struct{}) (string, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("HEAD", srv.URL+"/trpc/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("expected empty body for HEAD request, got %d bytes", len(body))
	}
}

func TestMutationContentTypeJSON(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "createUser", func(ctx context.Context, input CreateUserInput) (User, error) {
		return User{ID: "1", Name: input.Name, Email: input.Email}, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/createUser", "application/json", strings.NewReader(`{"name":"Jane","email":"j@e.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMutationContentTypeInvalid(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "createUser", func(ctx context.Context, input CreateUserInput) (User, error) {
		return User{}, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/trpc/createUser", "text/plain", strings.NewReader(`{"name":"Jane"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", resp.StatusCode)
	}
}

func TestMutationContentTypeEmpty(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "createUser", func(ctx context.Context, input CreateUserInput) (User, error) {
		return User{ID: "1", Name: input.Name}, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/trpc/createUser", strings.NewReader(`{"name":"Jane"}`))
	// No Content-Type header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for empty Content-Type, got %d", resp.StatusCode)
	}
}
