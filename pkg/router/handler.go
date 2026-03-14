package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

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

	// WebSocket upgrade — handle before method dispatch
	if isWebSocketUpgrade(req) {
		r.handleWebSocket(w, req)
		return
	}

	// Extract procedure path by stripping the basePath prefix.
	// Some ServeMux implementations (Go 1.22+) may strip the prefix
	// before dispatching, so handle both cases.
	prefix := r.basePath + "/"
	path := strings.TrimPrefix(req.URL.Path, prefix)
	if path == req.URL.Path {
		// Prefix wasn't present — the mux likely already stripped it
		path = strings.TrimPrefix(req.URL.Path, "/")
	}

	// Serve built-in panel UI at basePath/panel
	if !r.disablePanel {
		if path == "panel" || strings.HasPrefix(path, "panel/") || path == "panel/" {
			r.servePanel(w, req)
			return
		}
		if path == "" {
			http.Redirect(w, req, r.basePath+"/panel", http.StatusTemporaryRedirect)
			return
		}
	}

	if path == "" {
		r.writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "invalid trpc path", http.StatusNotFound, "")
		return
	}

	// Check for batch and streaming mode
	isBatch := req.URL.Query().Get("batch") == "1"
	isStream := req.Header.Get("trpc-batch-mode") == "stream"

	r.logger.Info("%s %s (batch=%v stream=%v)", req.Method, path, isBatch, isStream)

	switch req.Method {
	case http.MethodGet:
		r.handleQuery(w, req, path, isBatch, isStream)
	case http.MethodPost:
		r.handleMutation(w, req, path, isBatch, isStream)
	case http.MethodHead:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	default:
		r.writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "method not allowed", http.StatusMethodNotAllowed, path)
	}
}

func (r *Router) handleQuery(w http.ResponseWriter, req *http.Request, path string, isBatch bool, isStream bool) {
	if isBatch {
		names := strings.Split(path, ",")
		inputRaw := req.URL.Query().Get("input")
		var batchInput map[string]json.RawMessage
		if inputRaw != "" {
			if err := json.Unmarshal([]byte(inputRaw), &batchInput); err != nil {
				r.writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch input", http.StatusBadRequest, path)
				return
			}
		}
		getInput := func(i int) []byte {
			idx := strconv.Itoa(i)
			if raw, ok := batchInput[idx]; ok {
				return raw
			}
			return nil
		}
		if isStream {
			r.handleBatchStream(w, req, names, getInput)
		} else {
			r.handleBatch(w, req, names, getInput)
		}
		return
	}

	// Check if this is a subscription procedure
	if proc, ok := r.procedures[path]; ok && proc.Type == ProcedureSubscription {
		r.handleSubscription(w, req, path)
		return
	}

	// Single query
	inputRaw := req.URL.Query().Get("input")
	var inputJSON []byte
	if inputRaw != "" {
		inputJSON = []byte(inputRaw)
	}
	r.logger.Debug("query %s input=%s", path, string(inputJSON))

	result := r.callProcedure(w, req, path, ProcedureQuery, inputJSON)
	r.logger.Debug("query %s result=%+v", path, result)
	r.writeResult(w, result)
}

func (r *Router) handleMutation(w http.ResponseWriter, req *http.Request, path string, isBatch bool, isStream bool) {
	// Validate Content-Type for POST requests
	ct := req.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		r.writeErrorResponse(w, trpcerrors.ErrParseError, "unsupported Content-Type: expected application/json", http.StatusUnsupportedMediaType, path)
		return
	}

	defer func() { _ = req.Body.Close() }()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.writeErrorResponse(w, trpcerrors.ErrParseError, "failed to read body", http.StatusBadRequest, path)
		return
	}

	if isBatch {
		names := strings.Split(path, ",")
		// Batch mutation: body is an array or object keyed by index
		var batchInputs []json.RawMessage
		if len(body) > 0 {
			if err := json.Unmarshal(body, &batchInputs); err != nil {
				// Try as object keyed by index
				var batchMap map[string]json.RawMessage
				if err2 := json.Unmarshal(body, &batchMap); err2 != nil {
					r.writeErrorResponse(w, trpcerrors.ErrParseError, "failed to parse batch body", http.StatusBadRequest, path)
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
		getInput := func(i int) []byte {
			if i < len(batchInputs) {
				return batchInputs[i]
			}
			return nil
		}
		if isStream {
			r.handleBatchStream(w, req, names, getInput)
		} else {
			r.handleBatch(w, req, names, getInput)
		}
		return
	}

	// Single mutation
	var inputJSON []byte
	if len(body) > 0 {
		inputJSON = body
	}
	r.logger.Debug("mutation %s body=%s", path, string(inputJSON))

	result := r.callProcedure(w, req, path, ProcedureMutation, inputJSON)
	r.logger.Debug("mutation %s result=%+v", path, result)
	r.writeResult(w, result)
}

func (r *Router) callProcedure(w http.ResponseWriter, req *http.Request, name string, expectedType ProcedureType, inputJSON []byte) (result trpcResult) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("panic in procedure %s: %v\n%s", name, rec, debug.Stack())
			result = errorResult(trpcerrors.ErrInternalError, fmt.Sprintf("panic: %v", rec), name)
		}
	}()

	proc, ok := r.procedures[name]
	if !ok {
		return errorResult(trpcerrors.ErrMethodNotFound, "procedure not found: "+name, name)
	}

	if proc.Type != expectedType {
		return errorResult(trpcerrors.ErrMethodNotFound, "wrong method for procedure: "+name, name)
	}

	// Transformer: unwrap input envelope (e.g. superjson)
	transformed := false
	if r.transformer != nil && len(inputJSON) > 0 {
		plain, ok, err := r.transformer.TransformInput(inputJSON)
		if err != nil {
			return errorResult(trpcerrors.ErrParseError, "transformer input error: "+err.Error(), name)
		}
		inputJSON = plain
		transformed = ok
	}

	ctx := withRequest(req.Context(), req)
	if w != nil {
		ctx = withResponseWriter(ctx, w)
	}
	ctx = withProcedureName(ctx, name)

	handler := proc.Handler
	if len(proc.middlewares) > 0 {
		handler = applyMiddlewares(handler, proc.middlewares)
	}
	if len(r.middlewares) > 0 {
		handler = applyMiddlewares(handler, r.middlewares)
	}

	output, err := handler(ctx, Request{Input: inputJSON})
	if err != nil {
		return r.toErrorResult(err, name)
	}

	// Transformer: wrap output in envelope
	outputData := output
	if r.transformer != nil && transformed {
		wrapped, err := r.transformer.TransformOutput(output)
		if err != nil {
			return errorResult(trpcerrors.ErrInternalError, "transformer output error: "+err.Error(), name)
		}
		outputData = wrapped
	}

	return trpcResult{
		Result: &trpcData{
			Data: outputData,
		},
	}
}

func (r *Router) toErrorResult(err error, path string) trpcResult {
	code := trpcerrors.ErrInternalError
	msg := err.Error()

	if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
		code = trpcErr.Code
		msg = trpcErr.Message
		if trpcErr.Cause != nil {
			r.logger.Error("procedure %s error: %s (cause: %v)", path, msg, trpcErr.Cause)
		}
	}

	return errorResult(code, msg, path)
}

func (r *Router) writeErrorResponse(w http.ResponseWriter, code int, message string, httpStatus int, path string) {
	result := errorResult(code, message, path)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		r.logger.Error("failed to encode error response: %v", err)
	}
}

func (r *Router) writeResult(w http.ResponseWriter, result trpcResult) {
	w.Header().Set("Content-Type", "application/json")
	if result.Error != nil {
		w.WriteHeader(result.Error.Data.HTTPStatus)
	}
	if err := json.NewEncoder(w).Encode(result); err != nil {
		r.logger.Error("failed to encode response: %v", err)
	}
}

