package router_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestSubscriptionSSE(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
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

	resp, err := http.Get(srv.URL + "/trpc/counter")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read the full SSE stream
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Should contain 3 data events and a stopped event
	if !strings.Contains(content, "event: data") {
		t.Error("expected 'event: data' in SSE stream")
	}
	if !strings.Contains(content, "event: stopped") {
		t.Error("expected 'event: stopped' in SSE stream")
	}
	if !strings.Contains(content, `"data":0`) {
		t.Error("expected first event with data 0")
	}
	if !strings.Contains(content, `"data":2`) {
		t.Error("expected last event with data 2")
	}
}

func TestSubscriptionWithInput(t *testing.T) {
	type CountInput struct {
		Max int `json:"max"`
	}

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "countTo", func(ctx context.Context, input CountInput) (<-chan int, error) {
		ch := make(chan int)
		go func() {
			defer close(ch)
			for i := 1; i <= input.Max; i++ {
				ch <- i
			}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/countTo?input={"max":2}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, `"data":1`) {
		t.Error("expected event with data 1")
	}
	if !strings.Contains(content, `"data":2`) {
		t.Error("expected event with data 2")
	}
}

func TestSubscriptionError(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "failing", func(ctx context.Context, input struct{}) (<-chan string, error) {
		return nil, errors.New(errors.ErrUnauthorized, "not allowed")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/failing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Should return error as JSON, not SSE
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == nil {
		t.Error("expected error in response")
	}
}

func TestSubscriptionPanic(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "panicking", func(ctx context.Context, input struct{}) (<-chan string, error) {
		panic("subscription panic")
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/panicking")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestSubscriptionNestedRouter(t *testing.T) {
	eventsRouter := router.NewRouter()
	router.Subscription(eventsRouter, "stream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		go func() {
			defer close(ch)
			ch <- "hello"
		}()
		return ch, nil
	})

	appRouter := router.NewRouter(router.WithLogger(router.NopLogger))
	appRouter.Merge("events", eventsRouter)

	srv := httptest.NewServer(appRouter.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/events.stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, `"data":"hello"`) {
		t.Error("expected event with data 'hello'")
	}
}

func TestSubscriptionTrackedEvents(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "messages", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- router.TrackedEvent{ID: "msg-1", Data: "hello"}
			ch <- router.TrackedEvent{ID: "msg-2", Data: "world"}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// Should use custom IDs from TrackedEvent
	if !strings.Contains(content, "id: msg-1") {
		t.Error("expected tracked event id 'msg-1'")
	}
	if !strings.Contains(content, "id: msg-2") {
		t.Error("expected tracked event id 'msg-2'")
	}
	if !strings.Contains(content, `"data":"hello"`) {
		t.Error("expected data 'hello'")
	}
	if !strings.Contains(content, `"data":"world"`) {
		t.Error("expected data 'world'")
	}
}

func TestSubscriptionMixedTrackedAndUntracked(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "mixed", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- "plain"                                              // untracked, gets auto ID "0"
			ch <- router.TrackedEvent{ID: "custom-1", Data: "tracked"} // tracked
			ch <- "plain2"                                             // untracked, gets auto ID "1"
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/mixed")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	if !strings.Contains(content, "id: 0") {
		t.Error("expected auto-incremented id '0' for untracked event")
	}
	if !strings.Contains(content, "id: custom-1") {
		t.Error("expected tracked event id 'custom-1'")
	}
	if !strings.Contains(content, "id: 1") {
		t.Error("expected auto-incremented id '1' for second untracked event")
	}
}

func TestSubscriptionLastEventID(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "resumable", func(ctx context.Context, input struct{}) (<-chan interface{}, error) {
		lastID := router.GetLastEventID(ctx)
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- router.TrackedEvent{ID: "resume-from", Data: lastID}
		}()
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/trpc/resumable", nil)
	req.Header.Set("Last-Event-ID", "msg-42")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	// The handler should have received "msg-42" as the last event ID
	if !strings.Contains(content, `"data":"msg-42"`) {
		t.Errorf("expected handler to receive Last-Event-ID 'msg-42', got: %s", content)
	}
}

func TestSubscriptionGoroutineNoLeak(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	// Subscription that keeps emitting until context is cancelled
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

	// Connect and read a few events, then disconnect
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/trpc/infinite", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context cancellation may cause error — that's expected
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected SSE stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read until context expires
	io.ReadAll(resp.Body)
	// If the goroutine bridge leaks, this test will hang or the goroutine count will increase.
	// With -race flag, a leak would be detected as a stuck goroutine.
}

func TestSubscriptionProcedureNameInHandler(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	var capturedName string
	router.Subscription(r, "myStream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		capturedName = router.GetProcedureName(ctx)
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/myStream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if capturedName != "myStream" {
		t.Errorf("expected procedure name 'myStream' in subscription handler, got %q", capturedName)
	}
}

func TestSubscriptionProcedureNameInMiddleware(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	var capturedName string
	r.Use(func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			capturedName = router.GetProcedureName(ctx)
			return next(ctx, req)
		}
	})

	router.Subscription(r, "namedSub", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/trpc/namedSub")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if capturedName != "namedSub" {
		t.Errorf("expected procedure name 'namedSub' in middleware, got %q", capturedName)
	}
}

func TestSubscriptionInputValidation(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "validated", func(ctx context.Context, input ValidatedInput) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Empty name should fail validation
	resp, err := http.Get(srv.URL + `/trpc/validated?input={"name":""}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for subscription validation failure, got %d", resp.StatusCode)
	}
}

func TestSubscriptionInputMalformedJSON(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Subscription(r, "sub", func(ctx context.Context, input ValidatedInput) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + `/trpc/sub?input={bad-json}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON subscription input, got %d", resp.StatusCode)
	}
}

func TestSubscriptionWithTransformerInputError(t *testing.T) {
	r := router.NewRouter(
		router.WithLogger(router.NopLogger),
		router.WithTransformer(router.SuperJSONTransformer{}),
	)
	router.Subscription(r, "stream", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// Send malformed superjson envelope — transformer should fail
	resp, err := http.Get(srv.URL + `/trpc/stream?input={"json":INVALID}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 400 from transformer error, got %d: %s", resp.StatusCode, body)
	}
}
