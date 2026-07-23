package observability

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// TracingConfig controls optional OTLP/gRPC export.
type TracingConfig struct {
	Enabled     bool
	Endpoint    string
	Insecure    bool
	SampleRatio float64
	Service     string
	Version     string
	Environment string
}

// InitTracing installs an OTLP-backed global tracer provider when enabled and
// returns its bounded shutdown function. Disabled tracing has no exporter.
func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("observability: tracing enabled but OTLP endpoint is empty")
	}
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("observability: trace sample ratio must be between 0 and 1")
	}
	endpoint := normalizeOTLPEndpoint(cfg.Endpoint, cfg.Insecure)
	options := []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("observability: create OTLP exporter: %w", err)
	}
	res, err := traceResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("observability: create trace resource: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return provider.Shutdown, nil
}

func normalizeOTLPEndpoint(endpoint string, insecure bool) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	scheme := "https://"
	if insecure {
		scheme = "http://"
	}
	return scheme + endpoint
}

func traceResource(cfg TracingConfig) (*resource.Resource, error) {
	attributes := resource.NewWithAttributes(
		"",
		semconv.ServiceName(cfg.Service),
		semconv.ServiceVersion(cfg.Version),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	)
	return resource.Merge(resource.Default(), attributes)
}
