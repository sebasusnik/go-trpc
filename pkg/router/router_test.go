package router_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestPrintRoutes(t *testing.T) {
	var buf strings.Builder
	logger := router.LoggerFunc(func(format string, args ...any) {
		fmt.Fprintf(&buf, format+"\n", args...)
	})

	r := router.NewRouter(router.WithLogger(logger))
	router.Query(r, "getUser", func(ctx context.Context, input struct{}) (string, error) {
		return "", nil
	})
	router.Mutation(r, "createUser", func(ctx context.Context, input struct{}) (string, error) {
		return "", nil
	})
	router.Subscription(r, "onUpdate", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	r.PrintRoutes("/trpc")

	output := buf.String()

	// Verify all three procedure types
	if !strings.Contains(output, "query") || !strings.Contains(output, "getUser") {
		t.Errorf("expected query 'getUser' in output, got: %s", output)
	}
	if !strings.Contains(output, "mutation") || !strings.Contains(output, "createUser") {
		t.Errorf("expected mutation 'createUser' in output, got: %s", output)
	}
	if !strings.Contains(output, "subscription") || !strings.Contains(output, "onUpdate") {
		t.Errorf("expected subscription 'onUpdate' in output, got: %s", output)
	}

	// Verify HTTP methods
	if !strings.Contains(output, "GET") {
		t.Errorf("expected GET method in output, got: %s", output)
	}
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST method in output, got: %s", output)
	}

	// Verify base path
	if !strings.Contains(output, "/trpc/") {
		t.Errorf("expected base path '/trpc/' in output, got: %s", output)
	}

	// Verify alphabetical sorting: createUser < getUser < onUpdate
	createIdx := strings.Index(output, "createUser")
	getUserIdx := strings.Index(output, "getUser")
	onUpdateIdx := strings.Index(output, "onUpdate")
	if createIdx > getUserIdx || getUserIdx > onUpdateIdx {
		t.Error("expected procedures to be sorted alphabetically")
	}
}

func TestProcedures(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})
	router.Mutation(r, "create", func(ctx context.Context, input struct{}) (string, error) {
		return "", nil
	})
	router.Subscription(r, "events", func(ctx context.Context, input struct{}) (<-chan string, error) {
		ch := make(chan string)
		close(ch)
		return ch, nil
	})

	procs := r.Procedures()
	if len(procs) != 3 {
		t.Fatalf("expected 3 procedures, got %d", len(procs))
	}

	for _, name := range []string{"ping", "create", "events"} {
		if _, ok := procs[name]; !ok {
			t.Errorf("expected procedure %q to be registered", name)
		}
	}
}

func TestNewError(t *testing.T) {
	err := router.NewError(router.ErrForbidden, "access denied")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	trpcErr, ok := err.(*errors.TRPCError)
	if !ok {
		t.Fatalf("expected *errors.TRPCError, got %T", err)
	}

	if trpcErr.Code != errors.ErrForbidden {
		t.Errorf("expected code %d, got %d", errors.ErrForbidden, trpcErr.Code)
	}
	if trpcErr.Message != "access denied" {
		t.Errorf("expected message 'access denied', got %q", trpcErr.Message)
	}
}
