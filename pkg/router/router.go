package router

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Router is the main tRPC router that holds procedures and middlewares.
type Router struct {
	procedures   map[string]*procedure
	middlewares  []Middleware
	corsConfig   *CORSConfig
	transformer  Transformer
	logger       Logger
	basePath     string // URL prefix, e.g. "/trpc" (default)
	disablePanel bool
	panelOnce    sync.Once
	panelHandler http.Handler
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

// DisablePanel turns off the built-in panel UI.
// By default the panel is served at basePath/panel.
func DisablePanel() Option {
	return func(r *Router) {
		r.disablePanel = true
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
			InputType:           proc.InputType,
			OutputType:          proc.OutputType,
		}
	}
}

// Handler returns an http.Handler that serves the tRPC protocol.
func (r *Router) Handler() http.Handler {
	return r
}

// Procedures returns the registered procedures (used by codegen/schema).
func (r *Router) Procedures() map[string]*procedure {
	return r.procedures
}

// ProcedureInfos returns metadata for all registered procedures, sorted by name.
// This is the public API for tooling (playground, schema generators).
func (r *Router) ProcedureInfos() []ProcedureInfo {
	infos := make([]ProcedureInfo, 0, len(r.procedures))
	for _, p := range r.procedures {
		infos = append(infos, ProcedureInfo{
			Name:       p.Name,
			Type:       p.Type,
			InputType:  p.InputType,
			OutputType: p.OutputType,
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// BasePath returns the router's configured URL prefix.
func (r *Router) BasePath() string {
	return r.basePath
}
