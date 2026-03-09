package errors

import "fmt"

// tRPC error codes (TRPC_ERROR_CODES_BY_KEY)
const (
	ErrParseError      = -32700
	ErrInvalidRequest  = -32600
	ErrMethodNotFound  = -32601
	ErrInvalidParams   = -32602
	ErrInternalError   = -32603
	ErrUnauthorized    = -32001
	ErrForbidden       = -32003
	ErrNotFound        = -32004
	ErrTimeout         = -32008
	ErrConflict        = -32009
	ErrTooManyRequests = -32029
)

// ErrBadRequest is an alias for ErrInvalidRequest.
// Both map to tRPC code -32600. Use ErrBadRequest for application-level
// validation errors; ErrInvalidRequest for protocol-level errors.
const ErrBadRequest = ErrInvalidRequest

// codeNames maps error codes to their string representation.
var codeNames = map[int]string{
	ErrParseError:      "PARSE_ERROR",
	ErrInvalidRequest:  "BAD_REQUEST",
	ErrMethodNotFound:  "NOT_FOUND",
	ErrInvalidParams:   "BAD_REQUEST",
	ErrInternalError:   "INTERNAL_SERVER_ERROR",
	ErrUnauthorized:    "UNAUTHORIZED",
	ErrForbidden:       "FORBIDDEN",
	ErrNotFound:        "NOT_FOUND",
	ErrTimeout:         "TIMEOUT",
	ErrConflict:        "CONFLICT",
	ErrTooManyRequests: "TOO_MANY_REQUESTS",
}

// httpStatus maps error codes to HTTP status codes.
var httpStatus = map[int]int{
	ErrParseError:      400,
	ErrInvalidRequest:  400,
	ErrMethodNotFound:  404,
	ErrInvalidParams:   400,
	ErrInternalError:   500,
	ErrUnauthorized:    401,
	ErrForbidden:       403,
	ErrNotFound:        404,
	ErrTimeout:         408,
	ErrConflict:        409,
	ErrTooManyRequests: 429,
}

// TRPCError represents a tRPC-compatible error.
type TRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"` // original error for logging; never sent to client
}

func (e *TRPCError) Error() string {
	return fmt.Sprintf("trpc error %d: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As.
func (e *TRPCError) Unwrap() error {
	return e.Cause
}

// New creates a new TRPCError with the given code and message.
func New(code int, message string) *TRPCError {
	return &TRPCError{Code: code, Message: message}
}

// Newf creates a new TRPCError with a formatted message.
func Newf(code int, format string, args ...interface{}) *TRPCError {
	return &TRPCError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates a TRPCError that wraps an underlying error.
// The cause is preserved for logging but never sent to the client.
func Wrap(err error, code int, message string) *TRPCError {
	return &TRPCError{Code: code, Message: message, Cause: err}
}

// Wrapf creates a TRPCError that wraps an underlying error with a formatted message.
func Wrapf(err error, code int, format string, args ...interface{}) *TRPCError {
	return &TRPCError{Code: code, Message: fmt.Sprintf(format, args...), Cause: err}
}

// CodeName returns the string name for a tRPC error code.
func CodeName(code int) string {
	if name, ok := codeNames[code]; ok {
		return name
	}
	return "INTERNAL_SERVER_ERROR"
}

// HTTPStatus returns the HTTP status code for a tRPC error code.
func HTTPStatus(code int) int {
	if status, ok := httpStatus[code]; ok {
		return status
	}
	return 500
}
