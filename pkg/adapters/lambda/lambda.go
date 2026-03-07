package lambda

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// ToLambdaHandler converts a *gotrpc.Router into an AWS Lambda handler function
// compatible with API Gateway v2 (HTTP API) and Lambda Function URLs.
func ToLambdaHandler(r *gotrpc.Router) func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, apiReq events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		httpReq, err := toHTTPRequest(ctx, apiReq)
		if err != nil {
			return events.APIGatewayProxyResponse{StatusCode: 500, Body: `{"error":"failed to convert request"}`}, nil
		}

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httpReq)

		return toAPIGatewayResponse(rec), nil
	}
}

// Start is a convenience function that creates a Lambda handler from the router and starts it.
func Start(r *gotrpc.Router) {
	lambda.Start(ToLambdaHandler(r))
}

func toHTTPRequest(ctx context.Context, apiReq events.APIGatewayProxyRequest) (*http.Request, error) {
	url := apiReq.Path
	if len(apiReq.QueryStringParameters) > 0 {
		params := make([]string, 0, len(apiReq.QueryStringParameters))
		for k, v := range apiReq.QueryStringParameters {
			params = append(params, k+"="+v)
		}
		url += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequestWithContext(ctx, apiReq.HTTPMethod, url, strings.NewReader(apiReq.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range apiReq.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func toAPIGatewayResponse(rec *httptest.ResponseRecorder) events.APIGatewayProxyResponse {
	headers := make(map[string]string, len(rec.Header()))
	for k, v := range rec.Header() {
		headers[k] = strings.Join(v, ",")
	}
	return events.APIGatewayProxyResponse{
		StatusCode: rec.Code,
		Headers:    headers,
		Body:       rec.Body.String(),
	}
}
