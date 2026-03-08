package router

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey int

const (
	ctxKeyRequest contextKey = iota
	ctxKeyResponseWriter
	ctxKeyProcedureName
)

// withRequest stores the *http.Request in the context.
func withRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, ctxKeyRequest, r)
}

// withResponseWriter stores the http.ResponseWriter in the context.
func withResponseWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, ctxKeyResponseWriter, w)
}

// getResponseWriter returns the http.ResponseWriter from the context.
func getResponseWriter(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(ctxKeyResponseWriter).(http.ResponseWriter)
	return w
}

// withProcedureName stores the procedure name in the context.
func withProcedureName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ctxKeyProcedureName, name)
}

// GetProcedureName returns the current procedure name from the context.
func GetProcedureName(ctx context.Context) string {
	name, _ := ctx.Value(ctxKeyProcedureName).(string)
	return name
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

// GetClientIP extracts the client IP address from the request.
// It checks X-Forwarded-For, X-Real-IP, then falls back to RemoteAddr.
func GetClientIP(ctx context.Context) string {
	r := GetRequest(ctx)
	if r == nil {
		return ""
	}

	// Check X-Forwarded-For (first IP in the list)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (strip port)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// GetBearerToken extracts the bearer token from the Authorization header.
// Returns an empty string if the header is missing or not a Bearer token.
func GetBearerToken(ctx context.Context) string {
	auth := GetHeader(ctx, "Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// GetCookie returns the value of a named cookie, or an empty string if not found.
func GetCookie(ctx context.Context, name string) string {
	r := GetRequest(ctx)
	if r == nil {
		return ""
	}
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// GetQueryParam returns a URL query parameter value, or an empty string if not found.
func GetQueryParam(ctx context.Context, name string) string {
	r := GetRequest(ctx)
	if r == nil {
		return ""
	}
	return r.URL.Query().Get(name)
}

// SetHeader sets a response header from within a procedure handler.
// Headers must be set before the response body is written (which happens
// automatically after the handler returns).
func SetHeader(ctx context.Context, key, value string) {
	if w := getResponseWriter(ctx); w != nil {
		w.Header().Set(key, value)
	}
}

// AddHeader adds a response header value from within a procedure handler.
// Unlike SetHeader, this appends to existing values for the same key.
func AddHeader(ctx context.Context, key, value string) {
	if w := getResponseWriter(ctx); w != nil {
		w.Header().Add(key, value)
	}
}

// SetCookie sets a cookie on the response from within a procedure handler.
func SetCookie(ctx context.Context, cookie *http.Cookie) {
	if w := getResponseWriter(ctx); w != nil {
		http.SetCookie(w, cookie)
	}
}

// GetLastEventID returns the Last-Event-ID header sent by reconnecting SSE clients.
// Returns an empty string if the header is missing (first connection).
func GetLastEventID(ctx context.Context) string {
	return GetHeader(ctx, "Last-Event-ID")
}

// WithValue is a convenience wrapper for context.WithValue.
func WithValue(ctx context.Context, key, val interface{}) context.Context {
	return context.WithValue(ctx, key, val)
}
