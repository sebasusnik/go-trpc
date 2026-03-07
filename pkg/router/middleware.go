package router

// Middleware wraps a Handler, allowing pre/post processing.
type Middleware func(next Handler) Handler

// applyMiddlewares wraps a handler with all middlewares in order.
func applyMiddlewares(h Handler, middlewares []Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
