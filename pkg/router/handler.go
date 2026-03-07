package router

import (
	"encoding/json"
	"io"
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
	Data trpcJSON `json:"data"`
}

type trpcJSON struct {
	JSON interface{} `json:"json"`
}

type trpcError struct {
	JSON trpcErrorJSON `json:"json"`
}

type trpcErrorJSON struct {
	Message string        `json:"message"`
	Code    int           `json:"code"`
	Data    trpcErrorData `json:"data"`
}

type trpcErrorData struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus"`
	Path       string `json:"path"`
}

// tRPC input wrappers.
type trpcInputWrapper struct {
	JSON json.RawMessage `json:"json"`
}

type trpcBatchInput map[string]trpcInputWrapper

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
		var batchInput trpcBatchInput
		if inputRaw != "" {
			if err := json.Unmarshal([]byte(inputRaw), &batchInput); err != nil {
				writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch input", http.StatusBadRequest, path)
				return
			}
		}
		r.handleBatch(w, req, names, func(i int) []byte {
			idx := strconv.Itoa(i)
			if wrapper, ok := batchInput[idx]; ok {
				return wrapper.JSON
			}
			return nil
		})
		return
	}

	// Single query
	inputRaw := req.URL.Query().Get("input")
	var inputJSON []byte
	if inputRaw != "" {
		var wrapper trpcInputWrapper
		if err := json.Unmarshal([]byte(inputRaw), &wrapper); err != nil {
			writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse input", http.StatusBadRequest, path)
			return
		}
		inputJSON = wrapper.JSON
	}

	result := r.callProcedure(req, path, ProcedureQuery, inputJSON)
	writeJSON(w, result)
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
		// Batch mutation: body is an array of {json: ...} or object keyed by index
		var batchInputs []trpcInputWrapper
		if len(body) > 0 {
			if err := json.Unmarshal(body, &batchInputs); err != nil {
				// Try as object keyed by index
				var batchMap trpcBatchInput
				if err2 := json.Unmarshal(body, &batchMap); err2 != nil {
					writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch body", http.StatusBadRequest, path)
					return
				}
				for i := 0; i < len(names); i++ {
					idx := strconv.Itoa(i)
					if wrapper, ok := batchMap[idx]; ok {
						batchInputs = append(batchInputs, wrapper)
					} else {
						batchInputs = append(batchInputs, trpcInputWrapper{})
					}
				}
			}
		}
		r.handleBatch(w, req, names, func(i int) []byte {
			if i < len(batchInputs) {
				return batchInputs[i].JSON
			}
			return nil
		})
		return
	}

	// Single mutation
	var inputJSON []byte
	if len(body) > 0 {
		var wrapper trpcInputWrapper
		if err := json.Unmarshal(body, &wrapper); err != nil {
			writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse body", http.StatusBadRequest, path)
			return
		}
		inputJSON = wrapper.JSON
	}

	result := r.callProcedure(req, path, ProcedureMutation, inputJSON)
	writeJSON(w, result)
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
	writeJSON(w, results)
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
			Data: trpcJSON{JSON: result},
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
			JSON: trpcErrorJSON{
				Message: message,
				Code:    code,
				Data: trpcErrorData{
					Code:       trpcerrors.CodeName(code),
					HTTPStatus: trpcerrors.HTTPStatus(code),
					Path:       path,
				},
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
