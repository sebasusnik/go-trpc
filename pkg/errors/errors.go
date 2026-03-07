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
	ErrBadRequest      = -32600
)

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
}

func (e *TRPCError) Error() string {
	return fmt.Sprintf("trpc error %d: %s", e.Code, e.Message)
}

// New creates a new TRPCError with the given code and message.
func New(code int, message string) *TRPCError {
	return &TRPCError{Code: code, Message: message}
}

// Newf creates a new TRPCError with a formatted message.
func Newf(code int, format string, args ...interface{}) *TRPCError {
	return &TRPCError{Code: code, Message: fmt.Sprintf(format, args...)}
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
