package router

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (r *Router) handleBatch(w http.ResponseWriter, req *http.Request, names []string, getInput func(i int) []byte) {
	results := make([]trpcResult, len(names))
	for i, name := range names {
		name = strings.TrimSpace(name)
		inputJSON := getInput(i)

		// Determine procedure type from registration
		procType := ProcedureQuery
		if req.Method == http.MethodPost {
			procType = ProcedureMutation
		}
		results[i] = r.callProcedure(w, req, name, procType, inputJSON)
	}

	// Use 207 Multi-Status when batch contains mixed success/error results
	hasError := false
	hasSuccess := false
	for _, res := range results {
		if res.Error != nil {
			hasError = true
		} else {
			hasSuccess = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if hasError && !hasSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	} else if hasError && hasSuccess {
		w.WriteHeader(http.StatusMultiStatus)
	}
	if err := json.NewEncoder(w).Encode(results); err != nil {
		r.logger.Error("failed to encode batch response: %v", err)
	}
}
