package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init sets up OpenTelemetry tracing. Returns a shutdown function.
func Init(serviceName, exporterType string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if exporterType == "noop" || exporterType == "" {
		slog.Info("tracing disabled")
		return noop, nil
	}

	var exporter sdktrace.SpanExporter
	var err error

	switch exporterType {
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		return nil, fmt.Errorf("unsupported trace exporter: %s", exporterType)
	}
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	slog.Info("tracing initialized", "exporter", exporterType)
	return tp.Shutdown, nil
}
