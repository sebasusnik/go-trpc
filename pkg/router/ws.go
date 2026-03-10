package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"

	"github.com/coder/websocket"
)

// WebSocket wire types for tRPC wsLink protocol.

type wsRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type wsParams struct {
	Path  string          `json:"path"`
	Input json.RawMessage `json:"input,omitempty"`
}

type wsResponse struct {
	ID     int        `json:"id"`
	Result *wsResult  `json:"result,omitempty"`
	Error  *trpcError `json:"error,omitempty"`
}

type wsResult struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// wsConn manages a single WebSocket connection with multiplexed subscriptions.
type wsConn struct {
	router *Router
	conn   *websocket.Conn
	req    *http.Request

	mu   sync.Mutex // protects conn writes
	subs map[int]context.CancelFunc
	subsMu sync.Mutex
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// handleWebSocket accepts a WebSocket connection and runs the read loop.
func (r *Router) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		// Allow all origins — CORS is handled at the HTTP level by the router.
		InsecureSkipVerify: true,
	})
	if err != nil {
		r.logger.Error("websocket accept error: %v", err)
		return
	}

	wc := &wsConn{
		router: r,
		conn:   conn,
		req:    req,
		subs:   make(map[int]context.CancelFunc),
	}

	wc.readLoop(req.Context())
}

// readLoop reads JSON messages from the WebSocket and dispatches them.
func (wc *wsConn) readLoop(ctx context.Context) {
	defer func() {
		// Cancel all active subscriptions on disconnect.
		wc.subsMu.Lock()
		for id, cancel := range wc.subs {
			cancel()
			delete(wc.subs, id)
		}
		wc.subsMu.Unlock()

		_ = wc.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		_, data, err := wc.conn.Read(ctx)
		if err != nil {
			// Normal close or context cancelled — not an error.
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				wc.router.logger.Debug("websocket closed normally")
			} else if ctx.Err() != nil {
				wc.router.logger.Debug("websocket context cancelled")
			} else {
				wc.router.logger.Debug("websocket read error: %v", err)
			}
			return
		}

		var req wsRequest
		if err := json.Unmarshal(data, &req); err != nil {
			wc.writeJSON(wsResponse{
				ID: 0,
				Error: &trpcError{
					Message: "failed to parse message",
					Code:    trpcerrors.ErrParseError,
					Data: trpcErrorData{
						Code:       trpcerrors.CodeName(trpcerrors.ErrParseError),
						HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrParseError),
					},
				},
			})
			continue
		}

		switch req.Method {
		case "subscription":
			go wc.handleSubscriptionStart(ctx, req)
		case "subscription.stop":
			wc.handleSubscriptionStop(req)
		default:
			wc.writeJSON(wsResponse{
				ID: req.ID,
				Error: &trpcError{
					Message: fmt.Sprintf("unknown method: %s", req.Method),
					Code:    trpcerrors.ErrMethodNotFound,
					Data: trpcErrorData{
						Code:       trpcerrors.CodeName(trpcerrors.ErrMethodNotFound),
						HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrMethodNotFound),
					},
				},
			})
		}
	}
}

// handleSubscriptionStart starts a new subscription for the given request.
func (wc *wsConn) handleSubscriptionStart(connCtx context.Context, req wsRequest) {
	defer func() {
		if rec := recover(); rec != nil {
			wc.router.logger.Error("panic in ws subscription %d: %v\n%s", req.ID, rec, debug.Stack())
			wc.writeJSON(wsResponse{
				ID: req.ID,
				Error: &trpcError{
					Message: fmt.Sprintf("panic: %v", rec),
					Code:    trpcerrors.ErrInternalError,
					Data: trpcErrorData{
						Code:       trpcerrors.CodeName(trpcerrors.ErrInternalError),
						HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrInternalError),
					},
				},
			})
		}
	}()

	// Parse params
	var params wsParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			wc.writeJSON(wsResponse{
				ID: req.ID,
				Error: &trpcError{
					Message: "failed to parse params",
					Code:    trpcerrors.ErrParseError,
					Data: trpcErrorData{
						Code:       trpcerrors.CodeName(trpcerrors.ErrParseError),
						HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrParseError),
						Path:       params.Path,
					},
				},
			})
			return
		}
	}

	// Look up procedure
	proc, ok := wc.router.procedures[params.Path]
	if !ok {
		wc.writeJSON(wsResponse{
			ID: req.ID,
			Error: &trpcError{
				Message: "procedure not found: " + params.Path,
				Code:    trpcerrors.ErrMethodNotFound,
				Data: trpcErrorData{
					Code:       trpcerrors.CodeName(trpcerrors.ErrMethodNotFound),
					HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrMethodNotFound),
					Path:       params.Path,
				},
			},
		})
		return
	}

	if proc.Type != ProcedureSubscription {
		wc.writeJSON(wsResponse{
			ID: req.ID,
			Error: &trpcError{
				Message: "procedure is not a subscription: " + params.Path,
				Code:    trpcerrors.ErrMethodNotFound,
				Data: trpcErrorData{
					Code:       trpcerrors.CodeName(trpcerrors.ErrMethodNotFound),
					HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrMethodNotFound),
					Path:       params.Path,
				},
			},
		})
		return
	}

	// Parse input
	var inputJSON []byte
	if len(params.Input) > 0 {
		inputJSON = params.Input
	}

	// Transformer: unwrap input envelope
	if wc.router.transformer != nil && len(inputJSON) > 0 {
		plain, _, err := wc.router.transformer.TransformInput(inputJSON)
		if err != nil {
			wc.writeJSON(wsResponse{
				ID: req.ID,
				Error: &trpcError{
					Message: "transformer input error: " + err.Error(),
					Code:    trpcerrors.ErrParseError,
					Data: trpcErrorData{
						Code:       trpcerrors.CodeName(trpcerrors.ErrParseError),
						HTTPStatus: trpcerrors.HTTPStatus(trpcerrors.ErrParseError),
						Path:       params.Path,
					},
				},
			})
			return
		}
		inputJSON = plain
	}

	// Create cancellable context for this subscription
	subCtx, cancel := context.WithCancel(connCtx)

	// Register the subscription cancel func
	wc.subsMu.Lock()
	wc.subs[req.ID] = cancel
	wc.subsMu.Unlock()

	defer func() {
		cancel()
		wc.subsMu.Lock()
		delete(wc.subs, req.ID)
		wc.subsMu.Unlock()
	}()

	// Set up context with request info (for middlewares that read headers, IP, etc.)
	ctx := withRequest(subCtx, wc.req)
	ctx = withProcedureName(ctx, params.Path)

	// Call subscription handler (reuses the same middleware gate pattern as SSE)
	ch, err := wc.router.callSubscription(ctx, proc, inputJSON, params.Path)
	if err != nil {
		result := wc.router.toErrorResult(err, params.Path)
		wc.writeJSON(wsResponse{
			ID:    req.ID,
			Error: result.Error,
		})
		return
	}

	// Send "started" notification
	wc.writeJSON(wsResponse{
		ID:     req.ID,
		Result: &wsResult{Type: "started"},
	})

	wc.router.logger.Debug("ws subscription %d (%s) started", req.ID, params.Path)

	// Stream events from the channel.
	// When this loop exits (context cancel or channel close), we always send "stopped".
	defer func() {
		wc.writeJSON(wsResponse{
			ID:     req.ID,
			Result: &wsResult{Type: "stopped"},
		})
		wc.router.logger.Debug("ws subscription %d (%s) stopped", req.ID, params.Path)
	}()

	for {
		// Prioritize context cancellation over channel reads to ensure
		// prompt exit when subscription.stop is received.
		select {
		case <-subCtx.Done():
			return
		default:
		}

		select {
		case <-subCtx.Done():
			return
		case val, ok := <-ch:
			if !ok {
				return
			}

			// Unwrap TrackedEvent — over WebSocket we only use the Data, ignoring ID
			var eventData interface{}
			if tracked, isTracked := val.(TrackedEvent); isTracked {
				eventData = tracked.Data
			} else {
				eventData = val
			}

			wc.writeJSON(wsResponse{
				ID: req.ID,
				Result: &wsResult{
					Type: "data",
					Data: eventData,
				},
			})
		}
	}
}

// handleSubscriptionStop cancels an active subscription.
func (wc *wsConn) handleSubscriptionStop(req wsRequest) {
	wc.subsMu.Lock()
	cancel, ok := wc.subs[req.ID]
	wc.subsMu.Unlock()

	if ok {
		cancel()
		// The "stopped" response will be sent by handleSubscriptionStart's defer
		// when it detects the context cancellation and exits.
		wc.router.logger.Debug("ws subscription %d stop requested by client", req.ID)
	}
}

// writeJSON writes a JSON message to the WebSocket connection, protected by a mutex.
func (wc *wsConn) writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		wc.router.logger.Error("ws marshal error: %v", err)
		return
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	if err := wc.conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		wc.router.logger.Debug("ws write error: %v", err)
	}
}
