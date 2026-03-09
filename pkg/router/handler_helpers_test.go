package router_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/sebasusnik/go-trpc/pkg/errors"
	"github.com/sebasusnik/go-trpc/pkg/router"
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

func setupRouter() *router.Router {
	r := router.NewRouter()

	router.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			if input.ID == "not-found" {
				return User{}, errors.New(errors.ErrNotFound, "user not found")
			}
			return User{ID: input.ID, Name: "John", Email: "john@example.com"}, nil
		},
	)

	router.Mutation(r, "createUser",
		func(ctx context.Context, input CreateUserInput) (User, error) {
			return User{ID: "new-id", Name: input.Name, Email: input.Email}, nil
		},
	)

	return r
}

// captureLogger records log messages for test assertions.
type captureLogger struct {
	mu     sync.Mutex
	infos  []string
	debugs []string
	errors []string
}

func (l *captureLogger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, fmt.Sprintf(msg, args...))
}

func (l *captureLogger) Debug(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, fmt.Sprintf(msg, args...))
}

func (l *captureLogger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprintf(msg, args...))
}

// ValidatedInput is an input type implementing Validator for test assertions.
type ValidatedInput struct {
	Name string `json:"name"`
}

func (v *ValidatedInput) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// ValidatedInputCustomError is an input type that returns a TRPCError from Validate.
type ValidatedInputCustomError struct {
	Token string `json:"token"`
}

func (v *ValidatedInputCustomError) Validate() error {
	if v.Token == "" {
		return errors.New(errors.ErrUnauthorized, "missing token")
	}
	return nil
}

// TRPCValidatedInput returns different error types depending on the value.
type TRPCValidatedInput struct {
	Value string `json:"value"`
}

func (v *TRPCValidatedInput) Validate() error {
	if v.Value == "trpc-error" {
		return errors.New(errors.ErrForbidden, "forbidden by validator")
	}
	if v.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}
