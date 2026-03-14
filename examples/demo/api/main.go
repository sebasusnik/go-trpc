package main

import (
	"net/http"
	"os"

	"github.com/sebasusnik/go-trpc/examples/demo/api/app"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

func main() {
	r := app.NewRouter()

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	mux := http.NewServeMux()

	// Serve static files if /static directory exists (production Docker build),
	// otherwise redirect root to the playground panel.
	if info, err := os.Stat("/static"); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir("/static")))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/trpc/panel", http.StatusTemporaryRedirect)
		})
	}

	// tRPC API
	mux.Handle("/trpc/", r.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r.PrintRoutes("/trpc", ":"+port)
	http.ListenAndServe(":"+port, mux)
}
