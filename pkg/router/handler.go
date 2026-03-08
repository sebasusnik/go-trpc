package router

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// tRPC response envelope types.
type trpcResult struct {
	Result *trpcData  `json:"result,omitempty"`
	Error  *trpcError `json:"error,omitempty"`
}

type trpcData struct {
	Data interface{} `json:"data"`
}

type trpcError struct {
	Message string        `json:"message"`
	Code    int           `json:"code"`
	Data    trpcErrorData `json:"data"`
}

type trpcErrorData struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus"`
	Path       string `json:"path"`
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Handle CORS preflight
	if r.corsConfig != nil {
		r.writeCORSHeaders(w, req)
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// Extract procedure path
	path := strings.TrimPrefix(req.URL.Path, "/trpc/")
	if path == "" || path == req.URL.Path {
		writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "invalid trpc path", http.StatusNotFound, "")
		return
	}

	// Check for batch
	isBatch := req.URL.Query().Get("batch") == "1"

	log.Printf("[trpc] %s %s (batch=%v)\n", req.Method, path, isBatch)

	switch req.Method {
	case http.MethodGet:
		r.handleQuery(w, req, path, isBatch)
	case http.MethodPost:
		r.handleMutation(w, req, path, isBatch)
	default:
		writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "method not allowed", http.StatusMethodNotAllowed, path)
	}
}

func (r *Router) handleQuery(w http.ResponseWriter, req *http.Request, path string, isBatch bool) {
	if isBatch {
		names := strings.Split(path, ",")
		inputRaw := req.URL.Query().Get("input")
		var batchInput map[string]json.RawMessage
		if inputRaw != "" {
			if err := json.Unmarshal([]byte(inputRaw), &batchInput); err != nil {
				writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch input", http.StatusBadRequest, path)
				return
			}
		}
		r.handleBatch(w, req, names, func(i int) []byte {
			idx := strconv.Itoa(i)
			if raw, ok := batchInput[idx]; ok {
				return raw
			}
			return nil
		})
		return
	}

	// Single query
	inputRaw := req.URL.Query().Get("input")
	var inputJSON []byte
	if inputRaw != "" {
		inputJSON = []byte(inputRaw)
	}
	log.Printf("[trpc] query %s input=%s\n", path, string(inputJSON))

	result := r.callProcedure(req, path, ProcedureQuery, inputJSON)
	log.Printf("[trpc] query %s result=%+v\n", path, result)
	writeResult(w, result)
}

func (r *Router) handleMutation(w http.ResponseWriter, req *http.Request, path string, isBatch bool) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeErrorResponse(w, trpcerrors.ErrParseError, "failed to read body", http.StatusBadRequest, path)
		return
	}
	defer req.Body.Close()

	if isBatch {
		names := strings.Split(path, ",")
		// Batch mutation: body is an array or object keyed by index
		var batchInputs []json.RawMessage
		if len(body) > 0 {
			if err := json.Unmarshal(body, &batchInputs); err != nil {
				// Try as object keyed by index
				var batchMap map[string]json.RawMessage
				if err2 := json.Unmarshal(body, &batchMap); err2 != nil {
					writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch body", http.StatusBadRequest, path)
					return
				}
				for i := 0; i < len(names); i++ {
					idx := strconv.Itoa(i)
					if raw, ok := batchMap[idx]; ok {
						batchInputs = append(batchInputs, raw)
					} else {
						batchInputs = append(batchInputs, nil)
					}
				}
			}
		}
		r.handleBatch(w, req, names, func(i int) []byte {
			if i < len(batchInputs) {
				return batchInputs[i]
			}
			return nil
		})
		return
	}

	// Single mutation
	var inputJSON []byte
	if len(body) > 0 {
		inputJSON = body
	}
	log.Printf("[trpc] mutation %s body=%s\n", path, string(inputJSON))

	result := r.callProcedure(req, path, ProcedureMutation, inputJSON)
	log.Printf("[trpc] mutation %s result=%+v\n", path, result)
	writeResult(w, result)
}

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
		results[i] = r.callProcedure(req, name, procType, inputJSON)
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
	if hasError && hasSuccess {
		w.WriteHeader(http.StatusMultiStatus)
	}
	json.NewEncoder(w).Encode(results)
}

func (r *Router) callProcedure(req *http.Request, name string, expectedType ProcedureType, inputJSON []byte) trpcResult {
	proc, ok := r.procedures[name]
	if !ok {
		return errorResult(trpcerrors.ErrMethodNotFound, "procedure not found: "+name, name)
	}

	if proc.Type != expectedType {
		return errorResult(trpcerrors.ErrMethodNotFound, "wrong method for procedure: "+name, name)
	}

	ctx := withRequest(req.Context(), req)

	handler := proc.Handler
	if len(r.middlewares) > 0 {
		handler = applyMiddlewares(handler, r.middlewares)
	}

	result, err := handler(ctx, Request{Input: inputJSON})
	if err != nil {
		return toErrorResult(err, name)
	}

	return trpcResult{
		Result: &trpcData{
			Data: result,
		},
	}
}

func toErrorResult(err error, path string) trpcResult {
	code := trpcerrors.ErrInternalError
	msg := err.Error()

	if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
		code = trpcErr.Code
		msg = trpcErr.Message
	}

	return errorResult(code, msg, path)
}

func errorResult(code int, message, path string) trpcResult {
	return trpcResult{
		Error: &trpcError{
			Message: message,
			Code:    code,
			Data: trpcErrorData{
				Code:       trpcerrors.CodeName(code),
				HTTPStatus: trpcerrors.HTTPStatus(code),
				Path:       path,
			},
		},
	}
}

func writeErrorResponse(w http.ResponseWriter, code int, message string, httpStatus int, path string) {
	result := errorResult(code, message, path)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(result)
}

func writeResult(w http.ResponseWriter, result trpcResult) {
	w.Header().Set("Content-Type", "application/json")
	if result.Error != nil {
		w.WriteHeader(result.Error.Data.HTTPStatus)
	}
	json.NewEncoder(w).Encode(result)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (r *Router) writeCORSHeaders(w http.ResponseWriter, req *http.Request) {
	if r.corsConfig == nil {
		return
	}

	origin := req.Header.Get("Origin")
	if origin == "" {
		return
	}

	allowed := false
	for _, o := range r.corsConfig.AllowedOrigins {
		if o == "*" || o == origin {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")

	if len(r.corsConfig.AllowedMethods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(r.corsConfig.AllowedMethods, ", "))
	} else {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}

	if len(r.corsConfig.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(r.corsConfig.AllowedHeaders, ", "))
	}

	if r.corsConfig.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(r.corsConfig.MaxAge))
	}
}
