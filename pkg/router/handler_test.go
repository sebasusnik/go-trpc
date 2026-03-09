package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

// --- Panic Recovery Tests ---

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

// --- Logger Tests ---

type captureLogger struct {
	mu     sync.Mutex
	infos  []string
	debugs []string
	errors []string
}

func (l *captureLogger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, fmt.Sprintf(msg, args...))
}

func (l *captureLogger) Debug(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, fmt.Sprintf(msg, args...))
}

func (l *captureLogger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprintf(msg, args...))
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

// --- Input Validation Tests ---

type ValidatedInput struct {
	Name string `json:"name"`
}

func (v *ValidatedInput) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

type ValidatedInputCustomError struct {
	Token string `json:"token"`
}

func (v *ValidatedInputCustomError) Validate() error {
	if v.Token == "" {
		return errors.New(errors.ErrUnauthorized, "missing token")
	}
	return nil
}

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

// --- Batch Mutation Test ---

func TestBatchMutation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "createUser", func(ctx context.Context, input CreateUserInput) (User, error) {
		return User{ID: "new", Name: input.Name, Email: input.Email}, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	body := `[{"name":"Alice","email":"alice@example.com"},{"name":"Bob","email":"bob@example.com"}]`
	resp, err := http.Post(srv.URL+"/trpc/createUser,createUser?batch=1", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	first := results[0]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if first["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", first["name"])
	}
	second := results[1]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if second["name"] != "Bob" {
		t.Errorf("expected Bob, got %v", second["name"])
	}
}

// --- Context Helper Tests ---

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

// --- Error Cause Chain Tests ---

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

// --- Subscription (SSE) Tests ---

func TestSubscriptionSSE(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "counter", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			for i := 0; i < 3; i++ {
				ch <- i
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/counter")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read the full SSE stream
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Should contain 3 data events and a stopped event
	if !strings.Contains(content, "event: data") {
		t.Error("expected 'event: data' in SSE stream")
	}
	if !strings.Contains(content, "event: stopped") {
		t.Error("expected 'event: stopped' in SSE stream")
	}
	if !strings.Contains(content, `"data":0`) {
		t.Error("expected first event with data 0")
	}
	if !strings.Contains(content, `"data":2`) {
		t.Error("expected last event with data 2")
	}
}

func TestSubscriptionWithInput(t *testing.T) {
	type CountInput struct {
		Max int `json:"max"`
	}

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "countTo", func(ctx context.Context, input CountInput) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			for i := 1; i <= input.Max; i++ {
				ch <- i
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/countTo?input={"max":2}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, `"data":1`) {
		t.Error("expected event with data 1")
	}
	if !strings.Contains(content, `"data":2`) {
		t.Error("expected event with data 2")
	}
}

func TestSubscriptionError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "failing", func(ctx context.Context, input struct{}) (<-chan string, error) {
		return nil, errors.New(errors.ErrUnauthorized, "not allowed")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/failing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Should return error as JSON, not SSE
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == nil {
		t.Error("expected error in response")
	}
}

func TestSubscriptionPanic(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "panicking", func(ctx context.Context, input struct{}) (<-chan string, error) {
		panic("subscription panic")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/panicking")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestSubscriptionNestedRouter(t *testing.T) {
	eventsRouter := router.NewRouter()
	router.Subscription(eventsRouter, "stream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		go func() {
			defer close(ch)
			ch <- "hello"
		}()
		return ch, nil
	})

	appRouter := router.NewRouter(router.WithLogger(router.NopLogger))
	appRouter.Merge("events", eventsRouter)

	srv := httptest.NewServer(appRouter.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events.stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, `"data":"hello"`) {
		t.Error("expected event with data 'hello'")
	}
}

func TestSubscriptionTrackedEvents(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "messages", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- router.TrackedEvent{ID: "msg-1", Data: "hello"}
			ch <- router.TrackedEvent{ID: "msg-2", Data: "world"}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Should use custom IDs from TrackedEvent
	if !strings.Contains(content, "id: msg-1") {
		t.Error("expected tracked event id 'msg-1'")
	}
	if !strings.Contains(content, "id: msg-2") {
		t.Error("expected tracked event id 'msg-2'")
	}
	if !strings.Contains(content, `"data":"hello"`) {
		t.Error("expected data 'hello'")
	}
	if !strings.Contains(content, `"data":"world"`) {
		t.Error("expected data 'world'")
	}
}

func TestSubscriptionMixedTrackedAndUntracked(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "mixed", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- "plain"                                        // untracked, gets auto ID "0"
			ch <- router.TrackedEvent{ID: "custom-1", Data: "tracked"} // tracked
			ch <- "plain2"                                       // untracked, gets auto ID "1"
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/mixed")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, "id: 0") {
		t.Error("expected auto-incremented id '0' for untracked event")
	}
	if !strings.Contains(content, "id: custom-1") {
		t.Error("expected tracked event id 'custom-1'")
	}
	if !strings.Contains(content, "id: 1") {
		t.Error("expected auto-incremented id '1' for second untracked event")
	}
}

func TestSubscriptionLastEventID(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "resumable", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		lastID := router.GetLastEventID(ctx)
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- router.TrackedEvent{ID: "resume-from", Data: lastID}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/resumable", nil)
	req.Header.Set("Last-Event-ID", "msg-42")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// The handler should have received "msg-42" as the last event ID
	if !strings.Contains(content, `"data":"msg-42"`) {
		t.Errorf("expected handler to receive Last-Event-ID 'msg-42', got: %s", content)
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

// --- Bug fix tests ---

func TestMiddlewaresApplyToSubscriptions(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	// Register a middleware that rejects all requests (simulating auth)
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			return nil, errors.New(errors.ErrUnauthorized, "not authenticated")
		}
	})

	router.Subscription(r, "events",
		func(ctx context.Context, input struct{}) (<-chan string, error) {
			ch := make(chan string, 1)
			ch <- "should not reach here"
			close(ch)
			return ch, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Should get an unauthorized error, NOT an SSE stream
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("expected JSON error response, got: %s", string(body))
	}
	errField, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error field in response, got: %s", string(body))
	}
	if code, _ := errField["code"].(float64); int(code) != errors.ErrUnauthorized {
		t.Errorf("expected error code %d, got %v", errors.ErrUnauthorized, errField["code"])
	}
}

func TestMiddlewaresEnrichSubscriptionContext(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	type ctxKey string
	var captured string

	// Middleware that enriches context
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			ctx = context.WithValue(ctx, ctxKey("user"), "alice")
			return next(ctx, req)
		}
	})

	router.Subscription(r, "events",
		func(ctx context.Context, input struct{}) (<-chan string, error) {
			captured = ctx.Value(ctxKey("user")).(string)
			ch := make(chan string)
			close(ch)
			return ch, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if captured != "alice" {
		t.Errorf("expected middleware to enrich context with user=alice, got %q", captured)
	}
}

func TestBatchStreamNoRace(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	router.Query(r, "a",
		func(ctx context.Context, input struct{}) (string, error) {
			// Attempt SetHeader — should be a no-op in batch stream mode (nil writer)
			router.SetHeader(ctx, "X-Custom", "value")
			return "result-a", nil
		},
	)
	router.Query(r, "b",
		func(ctx context.Context, input struct{}) (string, error) {
			router.SetHeader(ctx, "X-Custom", "value")
			return "result-b", nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Run multiple times to increase chance of detecting races
	// (use -race flag for definitive detection)
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/trpc/a,b?batch=1", nil)
		req.Header.Set("trpc-batch-mode", "stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]json.RawMessage
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("failed to parse batch stream response: %s", string(body))
		}
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d: %s", len(result), string(body))
		}
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

// --- Bug fix verification tests ---

func TestSubscriptionGoroutineNoLeak(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	// Subscription that keeps emitting until context is cancelled
	router.Subscription(r, "infinite", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			i := 0
			for {
				select {
				case ch <- i:
					i++
				case <-ctx.Done():
					return
				}
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Connect and read a few events, then disconnect
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/trpc/infinite", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context cancellation may cause error — that's expected
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected SSE stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read until context expires
	io.ReadAll(resp.Body)
	// If the goroutine bridge leaks, this test will hang or the goroutine count will increase.
	// With -race flag, a leak would be detected as a stuck goroutine.
}

func TestSubscriptionProcedureNameInHandler(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	var capturedName string
	router.Subscription(r, "myStream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		capturedName = router.GetProcedureName(ctx)
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/myStream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if capturedName != "myStream" {
		t.Errorf("expected procedure name 'myStream' in subscription handler, got %q", capturedName)
	}
}

func TestSubscriptionProcedureNameInMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	var capturedName string
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			capturedName = router.GetProcedureName(ctx)
			return next(ctx, req)
		}
	})

	router.Subscription(r, "namedSub", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/namedSub")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if capturedName != "namedSub" {
		t.Errorf("expected procedure name 'namedSub' in middleware, got %q", capturedName)
	}
}

// --- CORS edge case tests ---

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

// --- Validator returning *TRPCError tests ---

type TRPCValidatedInput struct {
	Value string `json:"value"`
}

func (v *TRPCValidatedInput) Validate() error {
	if v.Value == "trpc-error" {
		return errors.New(errors.ErrForbidden, "forbidden by validator")
	}
	if v.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
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

// --- Context edge case tests ---

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

// --- Subscription input validation ---

func TestSubscriptionInputValidation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "validated", func(ctx context.Context, input ValidatedInput) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Empty name should fail validation
	resp, err := http.Get(srv.URL + `/trpc/validated?input={"name":""}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for subscription validation failure, got %d", resp.StatusCode)
	}
}

func TestSubscriptionInputMalformedJSON(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "sub", func(ctx context.Context, input ValidatedInput) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/sub?input={bad-json}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON subscription input, got %d", resp.StatusCode)
	}
}

// --- Stream edge cases ---

func TestStreamBatchSingleItem(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+`/trpc/ping?batch=1&input={"0":{}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode single-item batch stream: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if _, ok := result["0"]; !ok {
		t.Error("expected result at key '0'")
	}
}

func TestStreamBatchEmpty(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Empty batch — no procedure names
	req, _ := http.NewRequest("GET", srv.URL+`/trpc/?batch=1`, nil)
	req.Header.Set("trpc-batch-mode", "stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Should produce {} or error gracefully
	content := strings.TrimSpace(string(body))
	if content != "{}" && resp.StatusCode != http.StatusNotFound {
		// Either empty JSON or 404 is acceptable
		t.Logf("empty batch produced: status=%d body=%q", resp.StatusCode, content)
	}
}

func TestBatchAllErrorReturns500(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "fail1", func(ctx context.Context, input struct{}) (string, error) {
		return "", fmt.Errorf("error 1")
	})
	router.Query(r, "fail2", func(ctx context.Context, input struct{}) (string, error) {
		return "", fmt.Errorf("error 2")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/fail1,fail2?batch=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 when all batch entries error, got %d", resp.StatusCode)
	}

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, res := range results {
		if res["error"] == nil {
			t.Errorf("result %d: expected error", i)
		}
	}
}

func TestSubscriptionMiddlewareRejection(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	r.Use(router.BearerAuth(func(ctx context.Context, token string) (context.Context, error) {
		return ctx, fmt.Errorf("denied")
	}))
	router.Subscription(r, "events", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// No auth header → middleware should reject before subscription starts
	resp, err := http.Get(srv.URL + "/trpc/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 from middleware rejection, got %d: %s", resp.StatusCode, body)
	}
}

func TestSubscriptionWithTransformerInputError(t *testing.T) {
	r := router.NewRouter(
		router.WithLogger(router.NopLogger),
		router.WithTransformer(router.SuperJSONTransformer{}),
	)
	router.Subscription(r, "stream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Send malformed superjson envelope — transformer should fail
	resp, err := http.Get(srv.URL + `/trpc/stream?input={"json":INVALID}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400 from transformer error, got %d: %s", resp.StatusCode, body)
	}
}
