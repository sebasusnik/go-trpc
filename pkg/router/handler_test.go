package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

type GetUserInput struct {
	ID string `json:"id"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func setupRouter() *router.Router {
	r := router.NewRouter()

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			if input.ID == "not-found" {
				return User{}, errors.New(errors.ErrNotFound, "user not found")
			}
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	router.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	return r
}

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

func TestBatchQuery(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"2"}}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)

	if len(results) != 2 {
		t.Fatalf("expected 2 batch results, got %d", len(results))
	}

	first := results[0]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if first["id"] != "1" {
		t.Errorf("expected first id=1, got %v", first["id"])
	}

	second := results[1]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if second["id"] != "2" {
		t.Errorf("expected second id=2, got %v", second["id"])
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

func TestBatchMixedResults207(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// First query succeeds (id="1"), second fails (id="not-found")
	resp, err := http.Get(srv.URL + `/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"not-found"}}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("expected 207, got %d", resp.StatusCode)
	}
}

func TestBatchAllSuccess200(t *testing.T) {
	r := setupRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"2"}}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
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
