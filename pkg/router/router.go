package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// Router is the main tRPC router that holds procedures and middlewares.
type Router struct {
	procedures  map[string]*procedure
	middlewares []Middleware
	corsConfig  *CORSConfig
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{
		procedures: make(map[string]*procedure),
	}
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
			Name:    fullName,
			Type:    proc.Type,
			Handler: proc.Handler,
		}
	}
}

// Handler returns an http.Handler that serves the tRPC protocol.
func (r *Router) Handler() http.Handler {
	return r
}

// PrintRoutes prints all registered procedures to stdout.
func (r *Router) PrintRoutes(basePath string) {
	names := make([]string, 0, len(r.procedures))
	for name := range r.procedures {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("  Procedures:\n")
	for _, name := range names {
		proc := r.procedures[name]
		method := "GET"
		kind := "query"
		if proc.Type == ProcedureMutation {
			method = "POST"
			kind = "mutation"
		}
		fmt.Printf("    %-8s %-20s %s  %s/%s\n", kind, name, method, basePath, name)
	}
}

// Procedures returns the registered procedures (used by codegen/schema).
func (r *Router) Procedures() map[string]*procedure {
	return r.procedures
}

// Query registers a query procedure on the router.
func Query[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error)) {
	r.procedures[name] = &procedure{
		Name: name,
		Type: ProcedureQuery,
		Handler: func(ctx context.Context, req Request) (interface{}, error) {
			var input I
			if len(req.Input) > 0 {
				if err := json.Unmarshal(req.Input, &input); err != nil {
					return nil, trpcerrors.New(trpcerrors.ErrParseError, "failed to parse input: "+err.Error())
				}
			}
			return handler(ctx, input)
		},
	}
}

// Mutation registers a mutation procedure on the router.
func Mutation[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error)) {
	r.procedures[name] = &procedure{
		Name: name,
		Type: ProcedureMutation,
		Handler: func(ctx context.Context, req Request) (interface{}, error) {
			var input I
			if len(req.Input) > 0 {
				if err := json.Unmarshal(req.Input, &input); err != nil {
					return nil, trpcerrors.New(trpcerrors.ErrParseError, "failed to parse input: "+err.Error())
				}
			}
			return handler(ctx, input)
		},
	}
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
