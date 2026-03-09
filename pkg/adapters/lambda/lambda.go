package lambda

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	gotrpc "github.com/sebasusnik/go-trpc/pkg/router"
)

// ToLambdaHandler converts a *gotrpc.Router into an AWS Lambda handler function
// compatible with Lambda Function URLs and API Gateway v2 (HTTP API).
func ToLambdaHandler(r *gotrpc.Router) func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return func(ctx context.Context, apiReq events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		httpReq, err := toHTTPRequest(ctx, apiReq)
		if err != nil {
			return events.APIGatewayV2HTTPResponse{
				StatusCode: 500,
				Body:       fmt.Sprintf(`{"error":"failed to convert request: %s"}`, err.Error()),
			}, nil
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

func toHTTPRequest(ctx context.Context, apiReq events.APIGatewayV2HTTPRequest) (*http.Request, error) {
	url := apiReq.RawPath
	if apiReq.RawQueryString != "" {
		url += "?" + apiReq.RawQueryString
	}

	method := apiReq.RequestContext.HTTP.Method

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(apiReq.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range apiReq.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func toAPIGatewayResponse(rec *httptest.ResponseRecorder) events.APIGatewayV2HTTPResponse {
	headers := make(map[string]string, len(rec.Header()))
	for k, v := range rec.Header() {
		headers[k] = strings.Join(v, ",")
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: rec.Code,
		Headers:    headers,
		Body:       rec.Body.String(),
	}
}
