package router

import "context"

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

// procedure stores a registered tRPC procedure.
type procedure struct {
	Name                string
	Type                ProcedureType
	Handler             Handler             // for query/mutation
	SubscriptionHandler SubscriptionHandler // for subscription
}
