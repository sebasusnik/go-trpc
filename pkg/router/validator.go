package router

// Validator is an optional interface that input structs can implement
// to perform validation after JSON unmarshaling but before the handler.
// Return a *TRPCError to control the error code, or any error for BAD_REQUEST.
type Validator interface {
	Validate() error
}
