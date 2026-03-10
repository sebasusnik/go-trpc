package router

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// Version is the current version of go-trpc.
// It is set automatically from build info (go install / go get) or
// overridden via ldflags: -X github.com/sebasusnik/go-trpc/pkg/router.Version=v1.0.0
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	// When used as the main module (e.g. go run ./cmd/gotrpc)
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = v
		return
	}
	// When imported as a dependency
	for _, dep := range info.Deps {
		if dep.Path == "github.com/sebasusnik/go-trpc" {
			if dep.Version != "" {
				Version = dep.Version
			}
			return
		}
	}
}

// Router is the main tRPC router that holds procedures and middlewares.
type Router struct {
	procedures  map[string]*procedure
	middlewares []Middleware
	corsConfig  *CORSConfig
	transformer Transformer
	logger      Logger
	basePath    string // URL prefix, e.g. "/trpc" (default)
}

// Option configures a Router.
type Option func(*Router)

// WithTransformer sets the data transformer for the router.
// The transformer handles serialization formats like superjson.
// Implementations must be safe for concurrent use.
func WithTransformer(t Transformer) Option {
	return func(r *Router) {
		r.transformer = t
	}
}

// WithLogger sets the logger for the router.
// Pass NopLogger to disable logging entirely.
func WithLogger(l Logger) Option {
	return func(r *Router) {
		r.logger = l
	}
}

// WithBasePath sets the URL prefix for the router (default: "/trpc").
// The router will strip this prefix from incoming request paths.
func WithBasePath(path string) Option {
	return func(r *Router) {
		// Ensure it starts with / and doesn't end with /
		if path != "" && path[0] != '/' {
			path = "/" + path
		}
		path = strings.TrimRight(path, "/")
		r.basePath = path
	}
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// NewRouter creates a new Router with optional configuration.
func NewRouter(opts ...Option) *Router {
	r := &Router{
		procedures: make(map[string]*procedure),
		logger:     defaultLogger{},
		basePath:   "/trpc",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Use adds a middleware to the router.
func (r *Router) Use(m Middleware) {
	r.middlewares = append(r.middlewares, m)
}

// WithCORS configures CORS for the router.
func (r *Router) WithCORS(cfg CORSConfig) {
	r.corsConfig = &cfg
}

// Merge merges another router's procedures under a namespace prefix.
func (r *Router) Merge(prefix string, other *Router) {
	for name, proc := range other.procedures {
		fullName := prefix + "." + name
		r.procedures[fullName] = &procedure{
			Name:                fullName,
			Type:                proc.Type,
			Handler:             proc.Handler,
			SubscriptionHandler: proc.SubscriptionHandler,
			middlewares:         proc.middlewares,
		}
	}
}

// Handler returns an http.Handler that serves the tRPC protocol.
func (r *Router) Handler() http.Handler {
	return r
}

// PrintRoutes logs all registered procedures via the router's logger.
func (r *Router) PrintRoutes(basePath string) {
	names := make([]string, 0, len(r.procedures))
	for name := range r.procedures {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		proc := r.procedures[name]
		method := "GET"
		kind := "query"
		switch proc.Type {
		case ProcedureMutation:
			method = "POST"
			kind = "mutation"
		case ProcedureSubscription:
			kind = "subscription"
		}
		r.logger.Info("  %-8s %-20s %s  %s/%s", kind, name, method, basePath, name)
	}
}

// Procedures returns the registered procedures (used by codegen/schema).
func (r *Router) Procedures() map[string]*procedure {
	return r.procedures
}

// decodeAndValidate unmarshals JSON input and runs Validate() if implemented.
func decodeAndValidate[I any](input []byte) (I, error) {
	var v I
	if len(input) > 0 {
		if err := json.Unmarshal(input, &v); err != nil {
			return v, trpcerrors.New(trpcerrors.ErrParseError, "failed to parse input: "+err.Error())
		}
	}
	if val, ok := any(&v).(Validator); ok {
		if err := val.Validate(); err != nil {
			if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
				return v, trpcErr
			}
			return v, trpcerrors.New(trpcerrors.ErrBadRequest, err.Error())
		}
	}
	return v, nil
}

// registerProcedure is the shared implementation for Query and Mutation.
func registerProcedure[I any, O any](r *Router, name string, procType ProcedureType, handler func(ctx context.Context, input I) (O, error), opts []ProcedureOption) {
	p := &procedure{
		Name: name,
		Type: procType,
		Handler: func(ctx context.Context, req Request) (interface{}, error) {
			input, err := decodeAndValidate[I](req.Input)
			if err != nil {
				return nil, err
			}
			return handler(ctx, input)
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	r.procedures[name] = p
}

// Query registers a query procedure on the router.
func Query[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error), opts ...ProcedureOption) {
	registerProcedure(r, name, ProcedureQuery, handler, opts)
}

// Mutation registers a mutation procedure on the router.
func Mutation[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error), opts ...ProcedureOption) {
	registerProcedure(r, name, ProcedureMutation, handler, opts)
}

// Subscription registers a subscription procedure on the router.
// The handler returns a channel that yields events until closed.
// The channel is consumed via Server-Sent Events (SSE).
func Subscription[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (<-chan O, error), opts ...ProcedureOption) {
	p := &procedure{
		Name: name,
		Type: ProcedureSubscription,
		SubscriptionHandler: func(ctx context.Context, req Request) (<-chan interface{}, error) {
			input, err := decodeAndValidate[I](req.Input)
			if err != nil {
				return nil, err
			}
			ch, err := handler(ctx, input)
			if err != nil {
				return nil, err
			}
			// Bridge typed chan O to chan interface{}.
			// Use select on ctx.Done() to prevent goroutine leaks when
			// the SSE consumer disconnects but the producer keeps emitting.
			out := make(chan interface{})
			go func() {
				defer close(out)
				for v := range ch {
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				}
			}()
			return out, nil
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	r.procedures[name] = p
}

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
