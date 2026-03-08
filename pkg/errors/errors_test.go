package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	err := New(ErrNotFound, "user not found")
	if err.Code != ErrNotFound {
		t.Errorf("expected code %d, got %d", ErrNotFound, err.Code)
	}
	if err.Message != "user not found" {
		t.Errorf("expected message 'user not found', got %q", err.Message)
	}
	if err.Error() != "trpc error -32004: user not found" {
		t.Errorf("unexpected Error(): %q", err.Error())
	}
}

func TestCodeName(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{ErrNotFound, "NOT_FOUND"},
		{ErrUnauthorized, "UNAUTHORIZED"},
		{ErrForbidden, "FORBIDDEN"},
		{ErrInternalError, "INTERNAL_SERVER_ERROR"},
		{ErrParseError, "PARSE_ERROR"},
		{ErrTimeout, "TIMEOUT"},
		{ErrConflict, "CONFLICT"},
		{ErrTooManyRequests, "TOO_MANY_REQUESTS"},
		{-99999, "INTERNAL_SERVER_ERROR"}, // unknown code
	}

	for _, tt := range tests {
		got := CodeName(tt.code)
		if got != tt.want {
			t.Errorf("CodeName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestNewf(t *testing.T) {
	err := Newf(ErrBadRequest, "field %s is required", "email")
	if err.Code != ErrBadRequest {
		t.Errorf("expected code %d, got %d", ErrBadRequest, err.Code)
	}
	if err.Message != "field email is required" {
		t.Errorf("expected message 'field email is required', got %q", err.Message)
	}
}

func TestWrap(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := Wrap(cause, ErrInternalError, "database error")

	if err.Code != ErrInternalError {
		t.Errorf("expected code %d, got %d", ErrInternalError, err.Code)
	}
	if err.Message != "database error" {
		t.Errorf("expected message 'database error', got %q", err.Message)
	}
	if err.Cause != cause {
		t.Error("expected cause to be preserved")
	}
}

func TestWrapf(t *testing.T) {
	cause := fmt.Errorf("not found in cache")
	err := Wrapf(cause, ErrNotFound, "user %s not found", "alice")

	if err.Message != "user alice not found" {
		t.Errorf("expected formatted message, got %q", err.Message)
	}
	if err.Cause != cause {
		t.Error("expected cause to be preserved")
	}
}

func TestUnwrap(t *testing.T) {
	cause := fmt.Errorf("original error")
	err := Wrap(cause, ErrInternalError, "wrapped")

	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestErrorsIs(t *testing.T) {
	sentinel := fmt.Errorf("sentinel")
	err := Wrap(sentinel, ErrInternalError, "wrapped")

	if !errors.Is(err, sentinel) {
		t.Error("errors.Is should find the wrapped sentinel error")
	}
}

func TestUnwrapNilCause(t *testing.T) {
	err := New(ErrNotFound, "not found")
	if errors.Unwrap(err) != nil {
		t.Error("Unwrap() should return nil when no cause")
	}
}

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		code int
		want int
	}{
		{ErrNotFound, 404},
		{ErrUnauthorized, 401},
		{ErrForbidden, 403},
		{ErrInternalError, 500},
		{ErrTooManyRequests, 429},
		{-99999, 500}, // unknown code
	}

	for _, tt := range tests {
		got := HTTPStatus(tt.code)
		if got != tt.want {
			t.Errorf("HTTPStatus(%d) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
