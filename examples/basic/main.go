package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type GetUserInput struct {
	ID string `json:"id"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	r := gotrpc.NewRouter()

	// Query: GET /trpc/getUser?input={"id":"1"}
	gotrpc.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	// Mutation: POST /trpc/createUser  body: {"name":"Jane","email":"jane@example.com"}
	gotrpc.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	r.WithCORS(gotrpc.CORSConfig{
		AllowedOrigins: []string{"*"},
	})

	r.PrintRoutes("/trpc", ":8080")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
