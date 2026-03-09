package router

import (
	"net/http"
	"strconv"
	"strings"
)

func (r *Router) writeCORSHeaders(w http.ResponseWriter, req *http.Request) {
	if r.corsConfig == nil {
		return
	}

	origin := req.Header.Get("Origin")
	if origin == "" {
		return
	}

	allowed := false
	for _, o := range r.corsConfig.AllowedOrigins {
		if o == "*" || o == origin {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")

	if len(r.corsConfig.AllowedMethods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(r.corsConfig.AllowedMethods, ", "))
	} else {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}

	if len(r.corsConfig.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(r.corsConfig.AllowedHeaders, ", "))
	}

	if r.corsConfig.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(r.corsConfig.MaxAge))
	}
}
