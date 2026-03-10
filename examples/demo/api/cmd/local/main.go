package main

import (
	"fmt"
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

	fmt.Println("go-trpc chat demo running on http://localhost:8080")
	r.PrintRoutes("/trpc")

	http.ListenAndServe(":8080", r.Handler())
}
