package main

import (
	"context"
	"fmt"
	"net/http"

	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// User types
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

// Post types
type ListPostsInput struct {
	AuthorID string `json:"authorId"`
	Limit    int    `json:"limit,omitempty"`
}

type Post struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	AuthorID string `json:"authorId"`
}

func main() {
	// User router
	userRouter := gotrpc.NewRouter()
	gotrpc.Query(userRouter, "get",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)
	gotrpc.Mutation(userRouter, "create",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	// Post router
	postRouter := gotrpc.NewRouter()
	gotrpc.Query(postRouter, "list",
		func(ctx context.Context, input ListPostsInput) ([]Post, error) {
			return []Post{
				{ID: "1", Title: "Hello World", AuthorID: input.AuthorID},
			}, nil
		},
	)

	// Merge into app router
	appRouter := gotrpc.NewRouter()
	appRouter.Merge("user", userRouter)
	appRouter.Merge("post", postRouter)

	// Results in: /trpc/user.get, /trpc/user.create, /trpc/post.list
	fmt.Println("Server listening on :8080")
	http.ListenAndServe(":8080", appRouter.Handler())
}
