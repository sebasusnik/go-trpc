package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// CreateUserInput implements the Validator interface.
// go-trpc automatically calls Validate() after JSON unmarshaling.
type CreateUserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func (i *CreateUserInput) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if !strings.Contains(i.Email, "@") {
		return fmt.Errorf("invalid email address")
	}
	if i.Age < 0 || i.Age > 150 {
		// Return a TRPCError directly for custom error codes.
		return trpcerrors.Newf(trpcerrors.ErrBadRequest, "age must be between 0 and 150, got %d", i.Age)
	}
	return nil
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func main() {
	r := gotrpc.NewRouter()

	// go-trpc calls Validate() automatically after parsing input JSON.
	// If Validate() returns a plain error → BAD_REQUEST (-32600).
	// If Validate() returns a *trpcerrors.TRPCError → that code is used as-is.
	//
	// Try with invalid input:
	//   curl -X POST http://localhost:8080/trpc/createUser \
	//     -d '{"name":"","email":"bad","age":-1}'
	gotrpc.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{
				ID:    "new-id",
				Name:  input.Name,
				Email: input.Email,
				Age:   input.Age,
			}, nil
		},
	)

	r.PrintRoutes("/trpc", ":8080")
	fmt.Println("Server listening on :8080")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
