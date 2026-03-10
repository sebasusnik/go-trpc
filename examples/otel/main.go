package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sebasusnik/go-trpc/pkg/adapters/nethttp"
	gotrpcotel "github.com/sebasusnik/go-trpc/pkg/otel"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type GetUserInput struct {
	ID string `json:"id"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	// Set up a stdout trace exporter for demonstration.
	// In production, use an OTLP exporter (Jaeger, Tempo, etc.).
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()

	r := gotrpc.NewRouter()

	// The OTel middleware creates a span per procedure call and records
	// rpc.server.duration metrics. Each span includes:
	//   rpc.system = "trpc"
	//   rpc.method = <procedure name>
	r.Use(gotrpcotel.Middleware(
		gotrpcotel.WithTracerProvider(tp),
	))

	gotrpc.Query(r, "getUser",
		func(ctx context.Context, input GetUserInput) (User, error) {
			return User{ID: input.ID, Name: "Jane"}, nil
		},
	)

	r.PrintRoutes("/trpc")
	fmt.Println("Server listening on :8080 (traces → stdout)")
	srv := nethttp.NewServer(r, nethttp.Config{Addr: ":8080"})
	log.Fatal(srv.Start())
}
