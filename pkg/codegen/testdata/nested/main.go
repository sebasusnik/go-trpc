package nested

import "context"

type router struct{}

func (router) Query() {}

var gotrpc router

type Router struct{}

func (r *Router) Merge(prefix string, child *Router) {}

var r = &Router{}
var adminRouter = &Router{}

func setup() {
	gotrpc.Query(adminRouter, "listUsers", func(ctx context.Context, input struct{}) (string, error) {
		return "", nil
	})
	r.Merge("admin", adminRouter)
}
