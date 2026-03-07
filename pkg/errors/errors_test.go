package errors

import "testing"

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
