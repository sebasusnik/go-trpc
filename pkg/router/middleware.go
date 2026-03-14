package router

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
	"golang.org/x/time/rate"
)

// Middleware wraps a Handler, allowing pre/post processing.
type Middleware func(next Handler) Handler

// applyMiddlewares wraps a handler with all middlewares in order.
func applyMiddlewares(h Handler, middlewares []Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

type requestIDKeyType struct{}

// RequestIDKey is the context key for the request ID set by the RequestID middleware.
var RequestIDKey = requestIDKeyType{}

// RequestID returns a middleware that generates a unique request ID for each call.
// The ID is stored in context (retrieve via ctx.Value(router.RequestIDKey))
// and set as the X-Request-ID response header.
func RequestID() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			id := generateID()
			ctx = context.WithValue(ctx, RequestIDKey, id)
			SetHeader(ctx, "X-Request-ID", id)
			return next(ctx, req)
		}
	}
}

// LoggingMiddleware returns a middleware that logs each procedure call with its
// name, duration, and error status.
func LoggingMiddleware(logger Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			name := GetProcedureName(ctx)
			start := time.Now()
			result, err := next(ctx, req)
			duration := time.Since(start)
			if err != nil {
				logger.Error("proc %s failed (%s): %v", name, duration, err)
			} else {
				logger.Info("proc %s ok (%s)", name, duration)
			}
			return result, err
		}
	}
}

// BearerAuth returns a middleware that validates Bearer tokens from the
// Authorization header. The validate function receives the token and should
// return an enriched context (e.g. with user info) or an error.
func BearerAuth(validate func(ctx context.Context, token string) (context.Context, error)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			token := GetBearerToken(ctx)
			if token == "" {
				return nil, trpcerrors.New(trpcerrors.ErrUnauthorized, "missing or invalid authorization token")
			}
			newCtx, err := validate(ctx, token)
			if err != nil {
				if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
					return nil, trpcErr
				}
				return nil, trpcerrors.New(trpcerrors.ErrUnauthorized, err.Error())
			}
			return next(newCtx, req)
		}
	}
}

// APIKeyAuth returns a middleware that validates an API key from a custom header.
// The validate function receives the key value and should return an enriched
// context or an error.
func APIKeyAuth(header string, validate func(ctx context.Context, key string) (context.Context, error)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			key := GetHeader(ctx, header)
			if key == "" {
				return nil, trpcerrors.New(trpcerrors.ErrUnauthorized, "missing API key")
			}
			newCtx, err := validate(ctx, key)
			if err != nil {
				if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
					return nil, trpcErr
				}
				return nil, trpcerrors.New(trpcerrors.ErrUnauthorized, err.Error())
			}
			return next(newCtx, req)
		}
	}
}

// RateLimit returns a middleware that limits the number of requests per second
// across all procedures. Excess requests receive a TOO_MANY_REQUESTS error.
func RateLimit(requestsPerSecond int) Middleware {
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			if !limiter.Allow() {
				return nil, trpcerrors.New(trpcerrors.ErrTooManyRequests, "rate limit exceeded")
			}
			return next(ctx, req)
		}
	}
}

// MaxConnectionsPerIP returns a middleware that limits the number of concurrent
// requests from a single IP address. Useful for preventing a single client from
// exhausting server resources, especially with long-lived SSE subscriptions.
func MaxConnectionsPerIP(limit int) Middleware {
	var mu sync.Mutex
	counts := make(map[string]int)

	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			ip := GetClientIP(ctx)

			mu.Lock()
			if counts[ip] >= limit {
				mu.Unlock()
				return nil, trpcerrors.New(trpcerrors.ErrTooManyRequests,
					"too many concurrent connections from this IP")
			}
			counts[ip]++
			mu.Unlock()

			defer func() {
				mu.Lock()
				counts[ip]--
				if counts[ip] == 0 {
					delete(counts, ip)
				}
				mu.Unlock()
			}()

			return next(ctx, req)
		}
	}
}

// MaxInputSize returns a middleware that rejects requests whose input payload
// exceeds the given byte limit. Useful for preventing oversized messages.
func MaxInputSize(bytes int) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			if len(req.Input) > bytes {
				return nil, trpcerrors.New(trpcerrors.ErrBadRequest,
					fmt.Sprintf("input too large: %d bytes exceeds limit of %d", len(req.Input), bytes))
			}
			return next(ctx, req)
		}
	}
}

// RateLimitByKey returns a middleware that limits requests per second per key.
// The keyFunc extracts the rate-limiting key from the context (e.g. client IP, user ID).
// Inactive limiters are cleaned up periodically to prevent memory growth.
func RateLimitByKey(requestsPerSecond int, keyFunc func(ctx context.Context) string) Middleware {
	type entry struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var mu sync.Mutex
	limiters := make(map[string]*entry)

	// Cleanup goroutine: evict entries not seen in the last 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for k, e := range limiters {
				if e.lastSeen.Before(cutoff) {
					delete(limiters, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			key := keyFunc(ctx)

			mu.Lock()
			e, ok := limiters[key]
			if !ok {
				e = &entry{limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)}
				limiters[key] = e
			}
			e.lastSeen = time.Now()
			mu.Unlock()

			if !e.limiter.Allow() {
				return nil, trpcerrors.New(trpcerrors.ErrTooManyRequests, "rate limit exceeded")
			}
			return next(ctx, req)
		}
	}
}

// Timeout returns a middleware that cancels procedure execution after the given duration.
func Timeout(d time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req Request) (interface{}, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, req)
		}
	}
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
