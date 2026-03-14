package router

import (
	"context"
	"encoding/json"
	"reflect"

	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// decodeAndValidate unmarshals JSON input and runs Validate() if implemented.
func decodeAndValidate[I any](input []byte) (I, error) {
	var v I
	if len(input) > 0 {
		if err := json.Unmarshal(input, &v); err != nil {
			return v, trpcerrors.New(trpcerrors.ErrParseError, "failed to parse input: "+err.Error())
		}
	}
	if val, ok := any(&v).(Validator); ok {
		if err := val.Validate(); err != nil {
			if trpcErr, ok := err.(*trpcerrors.TRPCError); ok {
				return v, trpcErr
			}
			return v, trpcerrors.New(trpcerrors.ErrBadRequest, err.Error())
		}
	}
	return v, nil
}

// registerProcedure is the shared implementation for Query and Mutation.
func registerProcedure[I any, O any](r *Router, name string, procType ProcedureType, handler func(ctx context.Context, input I) (O, error), opts []ProcedureOption) {
	p := &procedure{
		Name:       name,
		Type:       procType,
		InputType:  reflect.TypeOf((*I)(nil)).Elem(),
		OutputType: reflect.TypeOf((*O)(nil)).Elem(),
		Handler: func(ctx context.Context, req Request) (interface{}, error) {
			input, err := decodeAndValidate[I](req.Input)
			if err != nil {
				return nil, err
			}
			return handler(ctx, input)
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	r.procedures[name] = p
}

// Query registers a query procedure on the router.
func Query[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error), opts ...ProcedureOption) {
	registerProcedure(r, name, ProcedureQuery, handler, opts)
}

// Mutation registers a mutation procedure on the router.
func Mutation[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (O, error), opts ...ProcedureOption) {
	registerProcedure(r, name, ProcedureMutation, handler, opts)
}

// Subscription registers a subscription procedure on the router.
// The handler returns a channel that yields events until closed.
// The channel is consumed via Server-Sent Events (SSE).
func Subscription[I any, O any](r *Router, name string, handler func(ctx context.Context, input I) (<-chan O, error), opts ...ProcedureOption) {
	p := &procedure{
		Name:       name,
		Type:       ProcedureSubscription,
		InputType:  reflect.TypeOf((*I)(nil)).Elem(),
		OutputType: reflect.TypeOf((*O)(nil)).Elem(),
		SubscriptionHandler: func(ctx context.Context, req Request) (<-chan interface{}, error) {
			input, err := decodeAndValidate[I](req.Input)
			if err != nil {
				return nil, err
			}
			ch, err := handler(ctx, input)
			if err != nil {
				return nil, err
			}
			// Bridge typed chan O to chan interface{}.
			// Use select on ctx.Done() to prevent goroutine leaks when
			// the SSE consumer disconnects but the producer keeps emitting.
			out := make(chan interface{})
			go func() {
				defer close(out)
				for v := range ch {
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				}
			}()
			return out, nil
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	r.procedures[name] = p
}
