package main

import (
	"context"

	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
)

// Input/output types — these become TypeScript types via gotrpc generate.

type GetUserInput struct {
	ID string `json:"id"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	r := gotrpc.NewRouter()

	gotrpc.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "Jane", Email: "jane@example.com"}, nil
		},
	)

	r.PrintRoutes("/trpc", ":8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	_ = srv.Start()
}
