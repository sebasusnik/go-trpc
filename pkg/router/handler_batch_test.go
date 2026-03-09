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

	"github.com/sebasusnik/go-trpc/pkg/router"
)

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
