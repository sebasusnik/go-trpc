package router

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	if _, err := fmt.Fprint(w, "{"); err != nil {
		r.logger.Error("batch stream write error: %v", err)
		return
	}
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
			// Pass nil writer: headers are already sent (WriteHeader called above),
			// so SetHeader from procedures is a no-op. This avoids a race on the
			// shared http.ResponseWriter between concurrent goroutines.
			result := r.callProcedure(nil, req, procName, procType, inputJSON)
			resultCh <- indexedResult{Index: idx, Result: result}
		}(i, name)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Stream results as they arrive
	var writeErr error
	first := true
	for ir := range resultCh {
		if writeErr != nil {
			continue // drain channel but skip writes
		}

		resultBytes, err := json.Marshal(ir.Result)
		if err != nil {
			r.logger.Error("batch stream marshal error: %v", err)
			continue
		}

		if !first {
			if _, writeErr = fmt.Fprint(w, ","); writeErr != nil {
				r.logger.Error("batch stream write error: %v", writeErr)
				continue
			}
		}
		if _, writeErr = fmt.Fprintf(w, "\"%d\":%s", ir.Index, resultBytes); writeErr != nil {
			r.logger.Error("batch stream write error: %v", writeErr)
			continue
		}
		flusher.Flush()
		first = false
	}

	// Write closing brace
	if writeErr == nil {
		if _, err := fmt.Fprint(w, "}"); err != nil {
			r.logger.Error("batch stream write error: %v", err)
		}
		flusher.Flush()
	}
}
