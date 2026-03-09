package simple

import "context"

type router struct{}

func (router) Query()    {}
func (router) Mutation() {}

var gotrpc router

type Router struct {
	procedures map[string]interface{}
}

func NewRouter() *Router { return &Router{} }

var r = NewRouter()

// These are the patterns ParseDir's AST parser looks for:
// gotrpc.Query(r, "name", handler) — SelectorExpr with "Query"/"Mutation"

func setup() {
	gotrpc.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})
	gotrpc.Mutation(r, "createItem", func(ctx context.Context, input struct{ Name string }) (string, error) {
		return "", nil
	})
}
