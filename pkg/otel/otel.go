// Package otel provides OpenTelemetry tracing and metrics integration for go-trpc.
//
// Usage:
//
//	r := router.NewRouter()
//	r.Use(otel.Middleware())
package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/sebasusnik/go-trpc/pkg/router"
)

const instrumentationName = "github.com/sebasusnik/go-trpc/pkg/otel"

type config struct {
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
}

// Option configures the OpenTelemetry middleware.
type Option func(*config)

// WithTracerProvider sets a custom tracer provider instead of the global default.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		c.tracerProvider = tp
	}
}

// WithMeterProvider sets a custom meter provider instead of the global default.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *config) {
		c.meterProvider = mp
	}
}

// Middleware returns a router.Middleware that creates OpenTelemetry spans
// and records metrics for each procedure call.
func Middleware(opts ...Option) router.Middleware {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	tp := cfg.tracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	tracer := tp.Tracer(instrumentationName)

	mp := cfg.meterProvider
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	meter := mp.Meter(instrumentationName)

	duration, err := meter.Float64Histogram(
		"rpc.server.duration",
		metric.WithDescription("Duration of tRPC procedure calls"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		otel.Handle(err)
	}

	return func(next router.Handler) router.Handler {
		return func(ctx context.Context, req router.Request) (interface{}, error) {
			name := router.GetProcedureName(ctx)
			spanName := "trpc." + name

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("rpc.system", "trpc"),
					attribute.String("rpc.method", name),
				),
			)
			defer span.End()

			start := time.Now()
			result, err := next(ctx, req)
			elapsed := float64(time.Since(start).Milliseconds())

			attrs := []attribute.KeyValue{
				attribute.String("rpc.method", name),
			}

			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
				attrs = append(attrs, attribute.Bool("rpc.error", true))
			} else {
				span.SetStatus(codes.Ok, "")
			}

			duration.Record(ctx, elapsed, metric.WithAttributes(attrs...))

			return result, err
		}
	}
}
