package main

import (
	"context"

	"github.com/sebasusnik/go-trpc/pkg/adapters/lambda"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

type GreetInput struct {
	Name string `json:"name"`
}

type GreetOutput struct {
	Message string `json:"message"`
}

func main() {
	r := gotrpc.NewRouter()

	gotrpc.Query(r, "greet",
		func(ctx context.Context, input GreetInput) (GreetOutput, error) {
			return GreetOutput{Message: "Hello, " + input.Name + "!"}, nil
		},
	)

	// lambda.Start converts the router to an AWS Lambda handler and starts it.
	// Compatible with Lambda Function URLs and API Gateway v2 (HTTP API).
	//
	// Deploy with:
	//   GOOS=linux GOARCH=arm64 go build -o bootstrap ./examples/lambda
	//   zip function.zip bootstrap
	//   aws lambda create-function --function-name my-trpc \
	//     --runtime provided.al2023 --architectures arm64 \
	//     --handler bootstrap --zip-file fileb://function.zip
	lambda.Start(r)
}
