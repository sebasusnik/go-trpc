package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// TrackedEvent wraps a subscription value with a custom event ID for
// tracked() semantics. When a subscription handler sends a TrackedEvent
// on its channel, the SSE stream uses the provided ID instead of an
// auto-incrementing counter. This enables clients to resume from where
// they left off via the Last-Event-ID header on reconnect.
//
// Usage in a handler:
//
//	ch <- router.TrackedEvent{ID: "msg-42", Data: myPayload}
type TrackedEvent struct {
	ID   string
	Data interface{}
}

// handleSubscription handles SSE subscription requests.
func (r *Router) handleSubscription(w http.ResponseWriter, req *http.Request, path string) {
	proc, ok := r.procedures[path]
	if !ok {
		writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "procedure not found: "+path, http.StatusNotFound, path)
		return
	}

	if proc.Type != ProcedureSubscription {
		writeErrorResponse(w, trpcerrors.ErrMethodNotFound, "procedure is not a subscription: "+path, http.StatusNotFound, path)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErrorResponse(w, trpcerrors.ErrInternalError, "streaming not supported", http.StatusInternalServerError, path)
		return
	}

	// Parse input from query param
	inputRaw := req.URL.Query().Get("input")
	var inputJSON []byte
	if inputRaw != "" {
		inputJSON = []byte(inputRaw)
	}

	// Transformer: unwrap input envelope
	if r.transformer != nil && len(inputJSON) > 0 {
		plain, _, err := r.transformer.TransformInput(inputJSON)
		if err != nil {
			writeErrorResponse(w, trpcerrors.ErrParseError, "transformer input error: "+err.Error(), http.StatusBadRequest, path)
			return
		}
		inputJSON = plain
	}

	ctx := withRequest(req.Context(), req)
	ctx = withResponseWriter(ctx, w)

	// Call subscription handler
	ch, err := r.callSubscription(ctx, proc, inputJSON, path)
	if err != nil {
		result := r.toErrorResult(err, path)
		writeResult(w, result)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	r.logger.Debug("subscription %s started", path)

	// Stream events
	eventID := 0
	for {
		select {
		case <-req.Context().Done():
			r.logger.Debug("subscription %s client disconnected", path)
			return
		case val, ok := <-ch:
			if !ok {
				// Channel closed — send stopped event
				fmt.Fprintf(w, "event: stopped\ndata: \n\n")
				flusher.Flush()
				r.logger.Debug("subscription %s stopped", path)
				return
			}

			// Check if the value is a TrackedEvent for custom event IDs
			var eventData interface{}
			var id string
			if tracked, isTracked := val.(TrackedEvent); isTracked {
				eventData = tracked.Data
				id = tracked.ID
			} else {
				eventData = val
				id = fmt.Sprintf("%d", eventID)
				eventID++
			}

			data, err := json.Marshal(trpcResult{
				Result: &trpcData{Data: eventData},
			})
			if err != nil {
				r.logger.Error("subscription %s marshal error: %v", path, err)
				continue
			}

			fmt.Fprintf(w, "id: %s\nevent: data\ndata: %s\n\n", id, data)
			flusher.Flush()
		}
	}
}

// callSubscription invokes the subscription handler with panic recovery and middleware.
func (r *Router) callSubscription(ctx context.Context, proc *procedure, inputJSON []byte, name string) (ch <-chan interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("panic in subscription %s: %v\n%s", name, rec, debug.Stack())
			ch = nil
			err = trpcerrors.New(trpcerrors.ErrInternalError, fmt.Sprintf("panic: %v", rec))
		}
	}()

	handler := proc.SubscriptionHandler

	return handler(ctx, Request{Input: inputJSON})
}
