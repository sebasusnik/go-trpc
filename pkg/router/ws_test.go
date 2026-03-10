package router_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

// wsResponse mirrors the server-side wsResponse for test decoding.
type wsResponse struct {
	ID     int              `json:"id"`
	Result *wsResultPayload `json:"result,omitempty"`
	Error  *wsErrorPayload  `json:"error,omitempty"`
}

type wsResultPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type wsErrorPayload struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// wsRequest is the client-side request message.
type wsRequest struct {
	ID     int         `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, srv.URL+"/trpc", nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	return conn
}

func wsSend(t *testing.T, conn *websocket.Conn, msg wsRequest) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal ws request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

func wsRead(t *testing.T, conn *websocket.Conn) wsResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var resp wsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal ws response: %v (raw: %s)", err, data)
	}
	return resp
}

func wsReadTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) (wsResponse, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsResponse{}, false
	}
	var resp wsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal ws response: %v (raw: %s)", err, data)
	}
	return resp, true
}

func setupWSRouter() *router.Router {
	return router.NewRouter(router.WithLogger(router.NopLogger))
}

func TestWS_SubscriptionStartData(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "counter", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			for i := 0; i < 3; i++ {
				ch <- i
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{
		ID:     1,
		Method: "subscription",
		Params: map[string]interface{}{"path": "counter"},
	})

	// Expect "started"
	resp := wsRead(t, conn)
	if resp.ID != 1 || resp.Result == nil || resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// Expect 3 data events
	for i := 0; i < 3; i++ {
		resp = wsRead(t, conn)
		if resp.ID != 1 || resp.Result == nil || resp.Result.Type != "data" {
			t.Fatalf("expected data event, got %+v", resp)
		}
		var val int
		json.Unmarshal(resp.Result.Data, &val)
		if val != i {
			t.Errorf("expected data %d, got %d", i, val)
		}
	}

	// Expect "stopped"
	resp = wsRead(t, conn)
	if resp.ID != 1 || resp.Result == nil || resp.Result.Type != "stopped" {
		t.Fatalf("expected stopped, got %+v", resp)
	}
}

func TestWS_SubscriptionStop(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "infinite", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			i := 0
			for {
				select {
				case ch <- i:
					i++
				case <-ctx.Done():
					return
				}
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{
		ID:     1,
		Method: "subscription",
		Params: map[string]interface{}{"path": "infinite"},
	})

	// Read started
	resp := wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// Read at least one data event
	resp = wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "data" {
		t.Fatalf("expected data, got %+v", resp)
	}

	// Send stop
	wsSend(t, conn, wsRequest{
		ID:     1,
		Method: "subscription.stop",
	})

	// Should get a "stopped" response (may be preceded by buffered data messages)
	gotStopped := false
	for i := 0; i < 50; i++ {
		resp, ok := wsReadTimeout(t, conn, 2*time.Second)
		if !ok {
			break
		}
		if resp.ID == 1 && resp.Result != nil && resp.Result.Type == "stopped" {
			gotStopped = true
			break
		}
	}
	if !gotStopped {
		t.Error("expected stopped response after subscription.stop")
	}
}

func TestWS_MultipleSubscriptions(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "letters", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		go func() {
			defer close(ch)
			ch <- "a"
			ch <- "b"
		}()
		return ch, nil
	})
	router.Subscription(r, "numbers", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			ch <- 1
			ch <- 2
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Start both subscriptions
	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "letters"}})
	wsSend(t, conn, wsRequest{ID: 2, Method: "subscription", Params: map[string]interface{}{"path": "numbers"}})

	// Collect all messages
	gotStarted := map[int]bool{}
	gotStopped := map[int]bool{}
	gotData := map[int]int{}

	for i := 0; i < 20; i++ {
		resp, ok := wsReadTimeout(t, conn, 3*time.Second)
		if !ok {
			break
		}
		if resp.Result != nil {
			switch resp.Result.Type {
			case "started":
				gotStarted[resp.ID] = true
			case "stopped":
				gotStopped[resp.ID] = true
			case "data":
				gotData[resp.ID]++
			}
		}
		if gotStopped[1] && gotStopped[2] {
			break
		}
	}

	if !gotStarted[1] || !gotStarted[2] {
		t.Error("expected both subscriptions to start")
	}
	if gotData[1] != 2 {
		t.Errorf("expected 2 data events for sub 1, got %d", gotData[1])
	}
	if gotData[2] != 2 {
		t.Errorf("expected 2 data events for sub 2, got %d", gotData[2])
	}
	if !gotStopped[1] || !gotStopped[2] {
		t.Error("expected both subscriptions to stop")
	}
}

func TestWS_SubscriptionStopCleansUp(t *testing.T) {
	var ctxCancelled atomic.Bool

	r := setupWSRouter()
	router.Subscription(r, "watched", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			<-ctx.Done()
			ctxCancelled.Store(true)
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "watched"}})

	// Wait for started
	resp := wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// Send stop
	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription.stop"})

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)

	if !ctxCancelled.Load() {
		t.Error("expected subscription context to be cancelled after stop")
	}
}

func TestWS_ConnectionClose(t *testing.T) {
	var ctxCancelled atomic.Bool

	r := setupWSRouter()
	router.Subscription(r, "longRunning", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			<-ctx.Done()
			ctxCancelled.Store(true)
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "longRunning"}})

	resp := wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// Close the WebSocket connection
	conn.Close(websocket.StatusNormalClosure, "bye")

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	if !ctxCancelled.Load() {
		t.Error("expected subscription context to be cancelled after connection close")
	}
}

func TestWS_InvalidMethod(t *testing.T) {
	r := setupWSRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "query"})

	resp := wsRead(t, conn)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != errors.ErrMethodNotFound {
		t.Errorf("expected METHOD_NOT_FOUND error code, got %d", resp.Error.Code)
	}
}

func TestWS_InvalidPath(t *testing.T) {
	r := setupWSRouter()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{
		ID:     1,
		Method: "subscription",
		Params: map[string]interface{}{"path": "nonExistent"},
	})

	resp := wsRead(t, conn)
	if resp.Error == nil {
		t.Fatal("expected error for non-existent procedure")
	}
	if resp.Error.Code != errors.ErrMethodNotFound {
		t.Errorf("expected METHOD_NOT_FOUND, got %d", resp.Error.Code)
	}
}

func TestWS_InvalidInput(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "validated", func(ctx context.Context, input ValidatedInput) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{
		ID:     1,
		Method: "subscription",
		Params: map[string]interface{}{
			"path":  "validated",
			"input": map[string]string{"name": ""},
		},
	})

	resp := wsRead(t, conn)
	if resp.Error == nil {
		t.Fatal("expected validation error")
	}
}

func TestWS_MiddlewareRuns(t *testing.T) {
	r := setupWSRouter()

	var middlewareCalled atomic.Bool
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			middlewareCalled.Store(true)
			return next(ctx, req)
		}
	})

	router.Subscription(r, "sub", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		go func() {
			defer close(ch)
			ch <- "hello"
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "sub"}})

	// Read started + data + stopped
	for i := 0; i < 3; i++ {
		wsRead(t, conn)
	}

	if !middlewareCalled.Load() {
		t.Error("expected global middleware to run for WebSocket subscription")
	}
}

func TestWS_ProcedureMiddleware(t *testing.T) {
	r := setupWSRouter()

	var procMiddlewareCalled atomic.Bool
	router.Subscription(r, "guarded", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	}, router.WithMiddleware(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			procMiddlewareCalled.Store(true)
			return next(ctx, req)
		}
	}))

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "guarded"}})

	// Read started + stopped
	for i := 0; i < 2; i++ {
		wsRead(t, conn)
	}

	if !procMiddlewareCalled.Load() {
		t.Error("expected procedure-level middleware to run for WebSocket subscription")
	}
}

func TestWS_TrackedEventUnwrap(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "tracked", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- router.TrackedEvent{ID: "msg-1", Data: "hello"}
			ch <- "plain"
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "tracked"}})

	// started
	resp := wsRead(t, conn)
	if resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// First data event — TrackedEvent should be unwrapped to just "hello"
	resp = wsRead(t, conn)
	if resp.Result.Type != "data" {
		t.Fatalf("expected data, got %+v", resp)
	}
	var val string
	json.Unmarshal(resp.Result.Data, &val)
	if val != "hello" {
		t.Errorf("expected TrackedEvent data 'hello', got %q", val)
	}

	// Second data event — plain string
	resp = wsRead(t, conn)
	var val2 string
	json.Unmarshal(resp.Result.Data, &val2)
	if val2 != "plain" {
		t.Errorf("expected plain data 'plain', got %q", val2)
	}

	// stopped
	resp = wsRead(t, conn)
	if resp.Result.Type != "stopped" {
		t.Fatalf("expected stopped, got %+v", resp)
	}
}

func TestWS_ConcurrentWrites(t *testing.T) {
	r := setupWSRouter()

	// Create 3 subscriptions that all emit concurrently
	for _, name := range []string{"s1", "s2", "s3"} {
		name := name
		_ = name
		router.Subscription(r, name, func(ctx context.Context, input struct{}) (<-chan int, error) {
			ch := make(chan int)
			go func() {
				defer close(ch)
				for i := 0; i < 10; i++ {
					select {
					case ch <- i:
					case <-ctx.Done():
						return
					}
				}
			}()
			return ch, nil
		})
	}

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Start all 3 subscriptions simultaneously
	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "s1"}})
	wsSend(t, conn, wsRequest{ID: 2, Method: "subscription", Params: map[string]interface{}{"path": "s2"}})
	wsSend(t, conn, wsRequest{ID: 3, Method: "subscription", Params: map[string]interface{}{"path": "s3"}})

	// Read all messages — if there's a race condition, -race flag will catch it
	stopped := map[int]bool{}
	for i := 0; i < 100; i++ {
		resp, ok := wsReadTimeout(t, conn, 3*time.Second)
		if !ok {
			break
		}
		if resp.Result != nil && resp.Result.Type == "stopped" {
			stopped[resp.ID] = true
		}
		if len(stopped) == 3 {
			break
		}
	}

	if len(stopped) != 3 {
		t.Errorf("expected all 3 subscriptions to stop, got %d", len(stopped))
	}
}

func TestWS_SubscriptionError(t *testing.T) {
	r := setupWSRouter()
	router.Subscription(r, "failing", func(ctx context.Context, input struct{}) (<-chan string, error) {
		return nil, errors.New(errors.ErrUnauthorized, "not allowed")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)
	defer conn.Close(websocket.StatusNormalClosure, "")

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "failing"}})

	resp := wsRead(t, conn)
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != errors.ErrUnauthorized {
		t.Errorf("expected UNAUTHORIZED error, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "not allowed" {
		t.Errorf("expected 'not allowed' message, got %q", resp.Error.Message)
	}
}

func TestWS_GracefulClose(t *testing.T) {
	r := setupWSRouter()

	var wg sync.WaitGroup
	wg.Add(1)

	router.Subscription(r, "slow", func(ctx context.Context, input struct{}) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			defer wg.Done()
			select {
			case ch <- 42:
			case <-ctx.Done():
				return
			}
			// Wait for context cancellation
			<-ctx.Done()
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	conn := dialWS(t, srv)

	wsSend(t, conn, wsRequest{ID: 1, Method: "subscription", Params: map[string]interface{}{"path": "slow"}})

	// Read started
	resp := wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "started" {
		t.Fatalf("expected started, got %+v", resp)
	}

	// Read data
	resp = wsRead(t, conn)
	if resp.Result == nil || resp.Result.Type != "data" {
		t.Fatalf("expected data, got %+v", resp)
	}

	// Close connection gracefully
	conn.Close(websocket.StatusNormalClosure, "done")

	// The subscription goroutine should clean up
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Error("subscription goroutine did not clean up after graceful close")
	}
}
