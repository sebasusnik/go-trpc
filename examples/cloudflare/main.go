package main

import (
	"context"

	"github.com/sebasusnik/go-trpc/pkg/adapters/cloudflare"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type PingOutput struct {
	Message string `json:"message"`
}

func main() {
	r := gotrpc.NewRouter()

	gotrpc.Query(r, "ping",
		func(ctx context.Context, input struct{}) (PingOutput, error) {
			return PingOutput{Message: "pong"}, nil
		},
	)

	// cloudflare.Serve starts the Workers WASM runtime with the router.
	// Uses syumai/workers under the hood for Go → WASM compilation.
	//
	// Build with:
	//   GOOS=js GOARCH=wasm go build -o app.wasm ./examples/cloudflare
	cloudflare.Serve(r)
}
