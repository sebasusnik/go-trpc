package router

import (
	"context"
	"reflect"
)

// ProcedureType is the type of a tRPC procedure.
type ProcedureType string

const (
	ProcedureQuery        ProcedureType = "query"
	ProcedureMutation     ProcedureType = "mutation"
	ProcedureSubscription ProcedureType = "subscription"
)

// Request holds the raw JSON input for a procedure call.
type Request struct {
	Input []byte
}

// Handler is the internal handler signature after type erasure.
type Handler func(ctx context.Context, req Request) (interface{}, error)

// SubscriptionHandler returns a channel that yields events until closed.
type SubscriptionHandler func(ctx context.Context, req Request) (<-chan interface{}, error)

// ProcedureOption configures a procedure at registration time.
type ProcedureOption func(*procedure)

// WithMiddleware attaches middlewares to a specific procedure.
// These run after global middlewares set via Router.Use().
func WithMiddleware(mws ...Middleware) ProcedureOption {
	return func(p *procedure) {
		p.middlewares = append(p.middlewares, mws...)
	}
}

// procedure stores a registered tRPC procedure.
type procedure struct {
	Name                string
	Type                ProcedureType
	Handler             Handler             // for query/mutation
	SubscriptionHandler SubscriptionHandler // for subscription
	middlewares         []Middleware         // procedure-level middlewares
	InputType           reflect.Type        // reflect.Type of the input parameter
	OutputType          reflect.Type        // reflect.Type of the output parameter
}

// ProcedureInfo exposes metadata about a registered procedure for tooling.
type ProcedureInfo struct {
	Name       string
	Type       ProcedureType
	InputType  reflect.Type
	OutputType reflect.Type
}

// Validator is an optional interface that input structs can implement
// to perform validation after JSON unmarshaling but before the handler.
// Return a *TRPCError to control the error code, or any error for BAD_REQUEST.
type Validator interface {
	Validate() error
}
