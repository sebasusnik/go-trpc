package router

import (
	"context"
	"net/http"
)

type contextKey int

const (
	ctxKeyRequest contextKey = iota
)

// withRequest stores the *http.Request in the context.
func withRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, ctxKeyRequest, r)
}

// GetRequest returns the original *http.Request from the context.
func GetRequest(ctx context.Context) *http.Request {
	r, _ := ctx.Value(ctxKeyRequest).(*http.Request)
	return r
}

// GetHeaders returns the request headers from the context.
func GetHeaders(ctx context.Context) http.Header {
	r := GetRequest(ctx)
	if r == nil {
		return http.Header{}
	}
	return r.Header
}

// GetHeader returns a single header value from the request.
func GetHeader(ctx context.Context, key string) string {
	return GetHeaders(ctx).Get(key)
}

// WithValue is a convenience wrapper for context.WithValue.
func WithValue(ctx context.Context, key, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
