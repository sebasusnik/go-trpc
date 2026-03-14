package router

import trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"

// NewError creates a new tRPC error (convenience re-export).
func NewError(code int, message string) error {
	return trpcerrors.New(code, message)
}

// Re-export error codes for convenience.
const (
	ErrUnauthorized = trpcerrors.ErrUnauthorized
	ErrForbidden    = trpcerrors.ErrForbidden
	ErrNotFound     = trpcerrors.ErrNotFound
	ErrBadRequest   = trpcerrors.ErrBadRequest
)
