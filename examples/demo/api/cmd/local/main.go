package main

import (
	"net/http"

	"github.com/sebasusnik/go-trpc/examples/demo/api/app"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

func main() {
	r := app.NewRouter()

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	mux := http.NewServeMux()
	mux.Handle("/trpc/", r.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/trpc/panel", http.StatusTemporaryRedirect)
	})

	r.PrintRoutes("/trpc", ":8080")

	http.ListenAndServe(":8080", mux)
}
