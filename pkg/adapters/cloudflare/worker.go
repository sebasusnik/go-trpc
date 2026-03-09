// Package cloudflare provides a CloudFlare Workers adapter for go-trpc.
//
// CloudFlare Workers with Go uses the syumai/workers library which provides
// a standard net/http compatible runtime via WASM.
//
// Usage:
//
//	r := router.NewRouter()
//	cloudflare.Serve(r)
package cloudflare

import (
	"net/http"

	"github.com/sebasusnik/go-trpc/pkg/router"
	"github.com/syumai/workers"
)

// Handler returns an http.Handler that serves the go-trpc router.
// Use this if you need to compose the handler with other middleware
// before passing to workers.Serve.
func Handler(r *router.Router) http.Handler {
	return r.Handler()
}

// Serve starts the CloudFlare Workers runtime with the given router.
// This function blocks and should be the last call in main().
func Serve(r *router.Router) {
	workers.Serve(Handler(r))
}
