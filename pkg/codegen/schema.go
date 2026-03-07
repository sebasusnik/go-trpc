package codegen

import (
	"encoding/json"
	"net/http"
)

// SchemaInfo describes a procedure for the introspection endpoint.
type SchemaInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // "query" or "mutation"
}

// SchemaResponse is the response for GET /trpc/__schema.
type SchemaResponse struct {
	Procedures []SchemaInfo `json:"procedures"`
}

// SchemaHandler returns an http.HandlerFunc that serves the introspection schema.
// It takes a list of procedure infos (typically from the router at startup).
func SchemaHandler(procedures []SchemaInfo) http.HandlerFunc {
	resp, _ := json.Marshal(SchemaResponse{Procedures: procedures})

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}
}
