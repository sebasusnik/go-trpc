package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// indexedResult pairs a batch index with its procedure result.
type indexedResult struct {
	Index  int
	Result trpcResult
}

// handleBatchStream runs procedures concurrently and streams results as they complete.
// The response format is a JSON object with string-indexed keys: {"0":{...},"1":{...}}.
// Each result is flushed immediately via chunked transfer encoding.
// Falls back to handleBatch if the ResponseWriter does not support http.Flusher.
func (r *Router) handleBatchStream(w http.ResponseWriter, req *http.Request, names []string, getInput func(i int) []byte) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		r.handleBatch(w, req, names, getInput)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Write opening brace
	fmt.Fprint(w, "{")
	flusher.Flush()

	resultCh := make(chan indexedResult, len(names))
	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)
		go func(idx int, procName string) {
			defer wg.Done()
			procName = strings.TrimSpace(procName)
			inputJSON := getInput(idx)

			procType := ProcedureQuery
			if req.Method == http.MethodPost {
				procType = ProcedureMutation
			}
			result := r.callProcedure(w, req, procName, procType, inputJSON)
			resultCh <- indexedResult{Index: idx, Result: result}
		}(i, name)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Stream results as they arrive
	first := true
	for ir := range resultCh {
		resultBytes, err := json.Marshal(ir.Result)
		if err != nil {
			continue
		}

		if !first {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, "%q:%s", strconv.Itoa(ir.Index), resultBytes)
		flusher.Flush()
		first = false
	}

	// Write closing brace
	fmt.Fprint(w, "}")
	flusher.Flush()
}
