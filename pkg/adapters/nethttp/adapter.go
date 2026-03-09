// Package nethttp provides a production-ready net/http adapter for go-trpc
// with configurable timeouts and graceful shutdown support.
//
// Usage:
//
//	r := router.NewRouter()
//	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
//	go srv.Start()
//	// ...
//	srv.Shutdown(context.Background())
package nethttp

import (
	"context"
	"net/http"
	"time"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

// Config configures the HTTP server.
type Config struct {
	Addr         string        // listen address, default ":8080"
	BasePath     string        // tRPC base path, default "/trpc"
	ReadTimeout  time.Duration // default 30s
	WriteTimeout time.Duration // default 30s
	IdleTimeout  time.Duration // default 120s
}

func (c *Config) defaults() {
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	if c.BasePath == "" {
		c.BasePath = "/trpc"
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 120 * time.Second
	}
}

// Server wraps a go-trpc Router in a production-ready *http.Server.
type Server struct {
	httpServer *http.Server
}

// NewServer creates a new Server that serves the given router.
func NewServer(r *router.Router, cfg Config) *Server {
	cfg.defaults()

	// Apply BasePath to the router so it strips the correct prefix.
	router.WithBasePath(cfg.BasePath)(r)

	mux := http.NewServeMux()
	mux.Handle("/", r.Handler())

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
}

// Start begins listening and serving HTTP requests.
// This blocks until the server is shut down. Returns http.ErrServerClosed
// on graceful shutdown.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// StartTLS begins listening and serving HTTPS requests.
func (s *Server) StartTLS(certFile, keyFile string) error {
	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
}

// Shutdown gracefully shuts down the server, waiting for in-flight requests
// to complete before returning.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
