package codegen_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/codegen"
)

func TestSchemaHandler(t *testing.T) {
	procedures := []codegen.SchemaInfo{
		{Name: "getUser", Type: "query"},
		{Name: "createUser", Type: "mutation"},
	}

	handler, err := codegen.SchemaHandler(procedures)
	if err != nil {
		t.Fatalf("SchemaHandler returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/__schema", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp codegen.SchemaResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Procedures) != 2 {
		t.Fatalf("expected 2 procedures, got %d", len(resp.Procedures))
	}

	if resp.Procedures[0].Name != "getUser" || resp.Procedures[0].Type != "query" {
		t.Errorf("unexpected first procedure: %+v", resp.Procedures[0])
	}
	if resp.Procedures[1].Name != "createUser" || resp.Procedures[1].Type != "mutation" {
		t.Errorf("unexpected second procedure: %+v", resp.Procedures[1])
	}
}

func TestSchemaHandlerEmpty(t *testing.T) {
	handler, err := codegen.SchemaHandler([]codegen.SchemaInfo{})
	if err != nil {
		t.Fatalf("SchemaHandler returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/__schema", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp codegen.SchemaResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Procedures == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(resp.Procedures) != 0 {
		t.Errorf("expected 0 procedures, got %d", len(resp.Procedures))
	}
}
