package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestStreamBatchQuery(t *testing.T) {
	r := router.NewRouter()

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+`/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"2"}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Response should be a JSON object with string-indexed keys
	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode streaming response: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Check result "0"
	r0, ok := result["0"]
	if !ok {
		t.Fatal("missing result at index 0")
	}
	r0Result := r0["result"].(map[string]interface{})
	r0Data := r0Result["data"].(map[string]interface{})
	if r0Data["id"] != "1" {
		t.Errorf("expected id=1, got %v", r0Data["id"])
	}

	// Check result "1"
	r1, ok := result["1"]
	if !ok {
		t.Fatal("missing result at index 1")
	}
	r1Result := r1["result"].(map[string]interface{})
	r1Data := r1Result["data"].(map[string]interface{})
	if r1Data["id"] != "2" {
		t.Errorf("expected id=2, got %v", r1Data["id"])
	}
}

func TestStreamBatchMutation(t *testing.T) {
	r := router.NewRouter()

	router.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	body := `[{"name":"Jane","email":"jane@example.com"},{"name":"Bob","email":"bob@example.com"}]`
	req, _ := http.NewRequest("POST", srv.URL+"/trpc/createUser,createUser?batch=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trpc-batch-mode", "stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode streaming response: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}

func TestStreamBatchConcurrentExecution(t *testing.T) {
	r := router.NewRouter()

	// "slow" procedure takes 100ms, "fast" takes 0ms
	router.Query(r, "slow",
		func(ctx context.Context, input struct{}) (map[string]string, error) {
			time.Sleep(100 * time.Millisecond)
			return map[string]string{"speed": "slow"}, nil
		},
	)
	router.Query(r, "fast",
		func(ctx context.Context, input struct{}) (map[string]string, error) {
			return map[string]string{"speed": "fast"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+`/trpc/slow,fast?batch=1&input={"0":{},"1":{}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	elapsed := time.Since(start)

	// Should complete in ~100ms (concurrent), not ~200ms (sequential)
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected concurrent execution (~100ms), took %v", elapsed)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Verify both results are present
	r0 := result["0"]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if r0["speed"] != "slow" {
		t.Errorf("expected slow, got %v", r0["speed"])
	}
	r1 := result["1"]["result"].(map[string]interface{})["data"].(map[string]interface{})
	if r1["speed"] != "fast" {
		t.Errorf("expected fast, got %v", r1["speed"])
	}
}

func TestStreamBatchMixedErrors(t *testing.T) {
	r := setupRouter() // has getUser that returns error for id="not-found"

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+`/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"not-found"}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Streaming always returns 200 (errors are in the body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// First should succeed
	if _, ok := result["0"]["result"]; !ok {
		t.Error("expected result at index 0")
	}

	// Second should be an error
	if _, ok := result["1"]["error"]; !ok {
		t.Error("expected error at index 1")
	}
}

func TestStreamBatchWithTransformer(t *testing.T) {
	r := router.NewRouter(router.WithTransformer(router.SuperJSONTransformer{}))

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+`/trpc/getUser,getUser?batch=1&input={"0":{"json":{"id":"1"},"meta":{}},"1":{"json":{"id":"2"},"meta":{}}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Results should be in superjson envelope
	for idx := range []string{"0", "1"} {
		key := strings.Repeat("", 0) + string(rune('0'+idx))
		r0 := result[key]["result"].(map[string]interface{})
		data := r0["data"].(map[string]interface{})
		if _, ok := data["json"]; !ok {
			t.Errorf("result %s: expected superjson envelope in output", key)
		}
	}
}

// noFlusherWriter wraps a ResponseWriter and strips the http.Flusher interface.
type noFlusherWriter struct {
	http.ResponseWriter
}

func TestStreamBatchFallbackNoFlusher(t *testing.T) {
	r := router.NewRouter()

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	// Use httptest.ResponseRecorder wrapped to strip Flusher
	req := httptest.NewRequest("GET", `/trpc/getUser,getUser?batch=1&input={"0":{"id":"1"},"1":{"id":"2"}}`, nil)
	req.Header.Set("trpc-batch-mode", "stream")

	w := &noFlusherWriter{httptest.NewRecorder()}
	r.Handler().ServeHTTP(w, req)

	recorder := w.ResponseWriter.(*httptest.ResponseRecorder)

	// Should fallback to array format (standard batch)
	var results []map[string]interface{}
	if err := json.NewDecoder(recorder.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode fallback response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results in array, got %d", len(results))
	}
}
