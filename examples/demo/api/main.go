package main

import (
	"fmt"
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

	// Serve static files if /static directory exists (production Docker build)
	if info, err := os.Stat("/static"); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir("/static")))
	}

	// tRPC API
	mux.Handle("/trpc/", r.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r.PrintRoutes("/trpc")
	fmt.Printf("go-trpc chat demo running on http://localhost:%s\n", port)
	http.ListenAndServe(":"+port, mux)
}
