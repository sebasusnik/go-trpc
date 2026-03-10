package router

import (
	trpcerrors "github.com/sebasusnik/go-trpc/pkg/errors"
)

// tRPC response envelope types.
type trpcResult struct {
	Result *trpcData  `json:"result,omitempty"`
	Error  *trpcError `json:"error,omitempty"`
}

type trpcData struct {
	Data interface{} `json:"data"`
}

type trpcError struct {
	Message string        `json:"message"`
	Code    int           `json:"code"`
	Data    trpcErrorData `json:"data"`
}

type trpcErrorData struct {
	Code       string  `json:"code"`
	HTTPStatus int     `json:"httpStatus"`
	Path       string  `json:"path"`
	Stack      *string `json:"stack,omitempty"`
}

func errorResult(code int, message, path string) trpcResult {
	return trpcResult{
		Error: &trpcError{
			Message: message,
			Code:    code,
			Data: trpcErrorData{
				Code:       trpcerrors.CodeName(code),
				HTTPStatus: trpcerrors.HTTPStatus(code),
				Path:       path,
			},
		},
	}
}
