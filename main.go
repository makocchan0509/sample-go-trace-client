package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"net/http"
	"os"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"log"
)

var tracer = otel.GetTracerProvider().Tracer("proto-ms-app-client/main")

type Entry struct {
	Message   string `json:"message"`
	Severity  string `json:"severity,omitempty"`
	Trace     string `json:"logging.googleapis.com/trace,omitempty"`
	SpanId    string `json:"logging.googleapis.com/spanId,omitempty"`
	Component string `json:"component,omitempty"`
}

// String renders an entry structure to the JSON format expected by Cloud Logging.
func (e Entry) String() string {
	if e.Severity == "" {
		e.Severity = "INFO"
	}
	out, err := json.Marshal(e)
	if err != nil {
		log.Printf("json.Marshal: %v", err)
	}
	return string(out)
}

func loadEnvFile() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Not found .env file: %v", err)
	}
}

func makeTraceIdFmt(traceId string) string {
	return fmt.Sprintf("projects/%s/traces/%s", os.Getenv("PROJECT_ID"), traceId)
}

func initTraceProvider(ctx context.Context, project string) *sdktrace.TracerProvider {
	exporter, err := texporter.New(texporter.WithProjectID(project))
	if err != nil {
		log.Fatalf("texporter.New: %v", err)
	}

	res, err := resource.New(ctx,
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("sample-client"),
		),
	)
	if err != nil {
		log.Fatalf("resource.New: %v", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
}

func main() {
	loadEnvFile()

	ctx := context.Background()
	project := os.Getenv("PROJECT_ID")
	endpoint := os.Getenv("ENDPOINT")

	log.SetFlags(0)

	tp := initTraceProvider(ctx, project)
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx1, span := tracer.Start(ctx, "requestServer")
	defer span.End()

	log.Println(Entry{
		Severity:  "INFO",
		Message:   "Request Server",
		Component: os.Getenv("APP_NAME"),
		Trace:     makeTraceIdFmt(span.SpanContext().TraceID().String()),
		SpanId:    span.SpanContext().SpanID().String(),
	})

	hc := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	hreq, err := http.NewRequestWithContext(ctx1, "GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := hc.Do(hreq)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
}
