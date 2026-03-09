package lambda_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/sebasusnik/go-trpc/pkg/adapters/lambda"
	"github.com/sebasusnik/go-trpc/pkg/router"
)

func TestToLambdaHandler_Query(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "ping", func(ctx context.Context, input struct{}) (string, error) {
		return "pong", nil
	})

	handler := lambda.ToLambdaHandler(r)

	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath:        "/trpc/ping",
		RawQueryString: "",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}
	if !json.Valid([]byte(resp.Body)) {
		t.Fatalf("expected valid JSON response, got: %s", resp.Body)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &result)
	resultField := result["result"].(map[string]interface{})
	if resultField["data"] != "pong" {
		t.Errorf("expected 'pong', got %v", resultField["data"])
	}
}

func TestToLambdaHandler_Mutation(t *testing.T) {
	type Input struct {
		Name string `json:"name"`
	}
	type Output struct {
		Greeting string `json:"greeting"`
	}

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Mutation(r, "greet", func(ctx context.Context, input Input) (Output, error) {
		return Output{Greeting: "Hello, " + input.Name}, nil
	})

	handler := lambda.ToLambdaHandler(r)

	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath: "/trpc/greet",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "POST",
			},
		},
		Headers: map[string]string{
			"content-type": "application/json",
		},
		Body: `{"name":"World"}`,
	})

	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &result)
	resultField := result["result"].(map[string]interface{})
	data := resultField["data"].(map[string]interface{})
	if data["greeting"] != "Hello, World" {
		t.Errorf("expected 'Hello, World', got %v", data["greeting"])
	}
}

func TestToLambdaHandler_QueryWithInput(t *testing.T) {
	type GetInput struct {
		ID string `json:"id"`
	}

	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "getItem", func(ctx context.Context, input GetInput) (string, error) {
		return "item-" + input.ID, nil
	})

	handler := lambda.ToLambdaHandler(r)

	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath:        "/trpc/getItem",
		RawQueryString: `input={"id":"42"}`,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, resp.Body)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &result)
	resultField := result["result"].(map[string]interface{})
	if resultField["data"] != "item-42" {
		t.Errorf("expected 'item-42', got %v", resultField["data"])
	}
}

func TestToLambdaHandler_NotFound(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))

	handler := lambda.ToLambdaHandler(r)

	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath: "/trpc/nonexistent",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d: %s", resp.StatusCode, resp.Body)
	}
}

func TestToLambdaHandler_HeadersForwarded(t *testing.T) {
	r := router.NewRouter(router.WithLogger(router.NopLogger))
	router.Query(r, "headers", func(ctx context.Context, input struct{}) (string, error) {
		return router.GetHeader(ctx, "X-Custom"), nil
	})

	handler := lambda.ToLambdaHandler(r)

	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath: "/trpc/headers",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
			},
		},
		Headers: map[string]string{
			"X-Custom": "my-value",
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &result)
	resultField := result["result"].(map[string]interface{})
	if resultField["data"] != "my-value" {
		t.Errorf("expected 'my-value', got %v", resultField["data"])
	}
}
