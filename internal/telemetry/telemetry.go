// Package telemetry ships errors and API timings to AppSignal over
// OpenTelemetry (OTLP/HTTP → the owner's hosted collector).
//
// The gate is absolute: a Reporter only goes live on a RELEASE build
// (version.IsRelease()) whose config.toml [telemetry] table carries both
// endpoint and key with enabled = true. Every other combination — source
// builds, unconfigured installs, opted-out installs, malformed endpoints —
// yields the inert zero Reporter: no goroutines, no network, no globals,
// and every method a cheap no-op. Telemetry must never be able to degrade
// the TUI; a bad config is silently inert, not an error the owner sees.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/gmrdad82/pito-tui/internal/config"
	"github.com/gmrdad82/pito-tui/internal/version"
)

// shutdownTimeout bounds the final flush so a ctrl+c exit never hangs on a
// slow collector.
const shutdownTimeout = 3 * time.Second

// Reporter owns the tracer provider. The zero value (and any gate-failing
// Init result) is inert and safe to call.
type Reporter struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

// Init builds a Reporter for cfg. Inert unless release + configured; a
// malformed endpoint is also inert by design (see package doc).
func Init(cfg config.Telemetry) *Reporter {
	if !version.IsRelease() || !cfg.Active() {
		return &Reporter{}
	}
	endpoint, err := tracesURL(cfg.Endpoint)
	if err != nil {
		return &Reporter{}
	}

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
	)
	if err != nil {
		return &Reporter{}
	}

	host, _ := os.Hostname()
	// AppSignal's collector reads app identity + auth from resource
	// attributes (appsignal.config.*), not headers.
	res := resource.NewSchemaless(
		attribute.String("service.name", "pito-tui"),
		attribute.String("host.name", host),
		attribute.String("appsignal.config.name", "pito-tui"),
		attribute.String("appsignal.config.environment", "production"),
		attribute.String("appsignal.config.push_api_key", cfg.Key),
		attribute.String("appsignal.config.language_integration", "go"),
		attribute.String("appsignal.config.revision", version.Commit),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return &Reporter{tp: tp, tracer: tp.Tracer("pito-tui")}
}

// Active reports whether spans actually leave the process.
func (r *Reporter) Active() bool {
	return r != nil && r.tp != nil
}

// Shutdown flushes and stops the provider, bounded by shutdownTimeout.
// Inert reporters return immediately.
func (r *Reporter) Shutdown() {
	if !r.Active() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = r.tp.Shutdown(ctx)
}

// ReportPanic records a crash as an error span, then flushes synchronously —
// the process is about to die, so the batcher can't be trusted to drain.
// The caller re-panics afterwards; reporting must never eat the crash.
func (r *Reporter) ReportPanic(v any, stack []byte) {
	if !r.Active() {
		return
	}
	_, span := r.tracer.Start(context.Background(), "panic")
	span.RecordError(fmt.Errorf("panic: %v", v))
	span.SetStatus(codes.Error, fmt.Sprintf("panic: %v", v))
	span.SetAttributes(attribute.String("exception.stacktrace", string(stack)))
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = r.tp.ForceFlush(ctx)
}

// Transport wraps base so every API request becomes a span (method, path,
// status, duration — never bodies, queries, headers, or cookies). Inert
// reporters return base untouched, so the wrap is safe to apply always.
func (r *Reporter) Transport(base http.RoundTripper) http.RoundTripper {
	if !r.Active() {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &roundTripper{base: base, tracer: r.tracer}
}

type roundTripper struct {
	base   http.RoundTripper
	tracer trace.Tracer
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := rt.tracer.Start(
		req.Context(),
		fmt.Sprintf("%s %s", req.Method, req.URL.Path),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", req.Method),
			attribute.String("url.path", req.URL.Path),
			attribute.String("server.address", req.URL.Hostname()),
		),
	)
	defer span.End()

	resp, err := rt.base.RoundTrip(req.WithContext(ctx))
	switch {
	case err != nil:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	case resp.StatusCode >= 500:
		span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
		span.SetStatus(codes.Error, resp.Status)
	default:
		span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	}
	return resp, err
}

// tracesURL normalizes the configured collector endpoint to the full OTLP
// traces URL: bare hosts gain https://, and the /v1/traces path is appended
// unless the owner already supplied a path.
func tracesURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("telemetry: empty endpoint")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("telemetry: %q is not a usable collector URL", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}
