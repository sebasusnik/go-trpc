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

func TestSuperJSONTransformInput_WithEnvelope(t *testing.T) {
	tr := router.SuperJSONTransformer{}
	raw := []byte(`{"json":{"id":"1"},"meta":{"values":{"id":["string"]}}}`)

	plain, transformed, err := tr.TransformInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !transformed {
		t.Fatal("expected transformed=true")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(plain, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["id"] != "1" {
		t.Errorf("expected id=1, got %v", parsed["id"])
	}
}

func TestSuperJSONTransformInput_PlainJSON(t *testing.T) {
	tr := router.SuperJSONTransformer{}
	raw := []byte(`{"id":"1"}`)

	plain, transformed, err := tr.TransformInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if transformed {
		t.Fatal("expected transformed=false for plain JSON")
	}
	if string(plain) != string(raw) {
		t.Errorf("expected pass-through, got %s", plain)
	}
}

func TestSuperJSONTransformInput_Empty(t *testing.T) {
	tr := router.SuperJSONTransformer{}

	plain, transformed, err := tr.TransformInput(nil)
	if err != nil {
		t.Fatal(err)
	}
	if transformed {
		t.Fatal("expected transformed=false for empty input")
	}
	if len(plain) != 0 {
		t.Errorf("expected empty, got %s", plain)
	}
}

func TestSuperJSONTransformOutput(t *testing.T) {
	tr := router.SuperJSONTransformer{}
	data := map[string]string{"id": "1", "name": "John"}

	result, err := tr.TransformOutput(data)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(bytes, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, ok := envelope["json"]; !ok {
		t.Fatal("expected json field in output envelope")
	}
	if _, ok := envelope["meta"]; !ok {
		t.Fatal("expected meta field in output envelope")
	}

	jsonField := envelope["json"].(map[string]interface{})
	if jsonField["id"] != "1" {
		t.Errorf("expected id=1, got %v", jsonField["id"])
	}
}

func TestQueryWithSuperJSON(t *testing.T) {
	r := router.NewRouter(router.WithTransformer(router.SuperJSONTransformer{}))

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Send superjson-wrapped input
	resp, err := http.Get(srv.URL + `/trpc/getUser?input={"json":{"id":"1"},"meta":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Output should be wrapped in superjson envelope
	resultField := result["result"].(map[string]interface{})
	data := resultField["data"].(map[string]interface{})

	jsonField, ok := data["json"].(map[string]interface{})
	if !ok {
		t.Fatal("expected superjson envelope in output (json field)")
	}
	if jsonField["id"] != "1" {
		t.Errorf("expected id=1, got %v", jsonField["id"])
	}
}

func TestQueryPlainJSONWithTransformerEnabled(t *testing.T) {
	r := router.NewRouter(router.WithTransformer(router.SuperJSONTransformer{}))

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Send plain JSON (no superjson envelope) — should pass through
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

	// Output should be plain (no superjson wrapping) since input was plain
	resultField := result["result"].(map[string]interface{})
	data := resultField["data"].(map[string]interface{})

	if data["id"] != "1" {
		t.Errorf("expected id=1, got %v", data["id"])
	}
	if _, hasJSON := data["json"]; hasJSON {
		t.Error("expected plain output, got superjson envelope")
	}
}

func TestMutationWithSuperJSON(t *testing.T) {
	r := router.NewRouter(router.WithTransformer(router.SuperJSONTransformer{}))

	router.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Send superjson-wrapped body
	body := `{"json":{"name":"Jane","email":"jane@example.com"},"meta":{}}`
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

	resultField := result["result"].(map[string]interface{})
	data := resultField["data"].(map[string]interface{})

	jsonField, ok := data["json"].(map[string]interface{})
	if !ok {
		t.Fatal("expected superjson envelope in output")
	}
	if jsonField["name"] != "Jane" {
		t.Errorf("expected name=Jane, got %v", jsonField["name"])
	}
}

func TestBatchWithSuperJSON(t *testing.T) {
	r := router.NewRouter(router.WithTransformer(router.SuperJSONTransformer{}))

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Batch with superjson-wrapped inputs
	resp, err := http.Get(srv.URL + `/trpc/getUser,getUser?batch=1&input={"0":{"json":{"id":"1"},"meta":{}},"1":{"json":{"id":"2"},"meta":{}}}`)
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

	for i, res := range results {
		data := res["result"].(map[string]interface{})["data"].(map[string]interface{})
		jsonField, ok := data["json"].(map[string]interface{})
		if !ok {
			t.Fatalf("result %d: expected superjson envelope", i)
		}
		if jsonField["name"] != "John" {
			t.Errorf("result %d: expected name=John, got %v", i, jsonField["name"])
		}
	}
}
