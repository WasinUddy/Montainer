package logging

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// OTLPConfig configures the optional OTLP/HTTP log sink. Endpoint must be the
// full signal URL (normally ending in /v1/logs). An empty endpoint disables
// the sink and NewOTLPSink returns nil, nil.
type OTLPConfig struct {
	Endpoint           string
	Protocol           string
	Insecure           bool
	ServiceName        string
	ServiceInstanceID  string
	ServiceVersion     string
	ResourceAttributes map[string]string
	QueueSize          int
	BatchSize          int
	ExportInterval     time.Duration
	RequestTimeout     time.Duration
	ExportTimeout      time.Duration
}

// OTLPSink bridges Montainer records into the OpenTelemetry Logs SDK. The SDK
// batch processor uses a bounded queue and performs network export outside the
// caller, so collector outages cannot block local/UI delivery.
type OTLPSink struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

func NewOTLPSink(ctx context.Context, cfg OTLPConfig) (*OTLPSink, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, nil
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "http/protobuf"
	}
	if cfg.Protocol != "http/protobuf" {
		return nil, fmt.Errorf("unsupported OTLP logs protocol %q", cfg.Protocol)
	}
	endpointURL, err := url.Parse(cfg.Endpoint)
	if err != nil || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") || endpointURL.Host == "" {
		return nil, fmt.Errorf("invalid OTLP logs endpoint %q", cfg.Endpoint)
	}
	if endpointURL.RawQuery != "" || endpointURL.Fragment != "" {
		return nil, fmt.Errorf("OTLP logs endpoint must not contain a query or fragment")
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "montainer"
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 2_048
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 512
	}
	if cfg.BatchSize > cfg.QueueSize {
		return nil, fmt.Errorf("OTLP batch size must not exceed queue size")
	}
	if cfg.ExportInterval <= 0 {
		cfg.ExportInterval = 5 * time.Second
	}
	if cfg.ExportTimeout <= 0 {
		cfg.ExportTimeout = 30 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}

	exporterOptions := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(cfg.Endpoint),
		otlploghttp.WithTimeout(cfg.RequestTimeout),
	}
	if cfg.Insecure {
		exporterOptions = append(exporterOptions, otlploghttp.WithInsecure())
	}
	exporter, err := otlploghttp.New(ctx, exporterOptions...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP logs exporter: %w", err)
	}

	resourceAttrs := make([]attribute.KeyValue, 0, len(cfg.ResourceAttributes)+3)
	keys := make([]string, 0, len(cfg.ResourceAttributes))
	for key := range cfg.ResourceAttributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		resourceAttrs = append(resourceAttrs, attribute.String(key, cfg.ResourceAttributes[key]))
	}
	// Append standard identity last so OTEL_SERVICE_NAME and INSTANCE_NAME
	// take precedence over duplicate generic resource attributes.
	resourceAttrs = append(resourceAttrs, attribute.String("service.name", cfg.ServiceName))
	if cfg.ServiceInstanceID != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("service.instance.id", cfg.ServiceInstanceID))
	}
	if cfg.ServiceVersion != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("service.version", cfg.ServiceVersion))
	}
	res := resource.NewWithAttributes("", resourceAttrs...)

	processor := sdklog.NewBatchProcessor(
		exporter,
		sdklog.WithMaxQueueSize(cfg.QueueSize),
		sdklog.WithExportMaxBatchSize(cfg.BatchSize),
		sdklog.WithExportInterval(cfg.ExportInterval),
		sdklog.WithExportTimeout(cfg.ExportTimeout),
	)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)
	return &OTLPSink{
		provider: provider,
		logger:   provider.Logger("github.com/wasinuddy/montainer/v2"),
	}, nil
}

func (s *OTLPSink) Write(ctx context.Context, record Record) error {
	if s == nil {
		return nil
	}
	var output otellog.Record
	output.SetTimestamp(record.Timestamp)
	output.SetObservedTimestamp(record.ObservedTimestamp)
	output.SetBody(otellog.StringValue(record.Body))

	attributes := make([]otellog.KeyValue, 0, len(record.Attributes)+1)
	keys := make([]string, 0, len(record.Attributes))
	for key := range record.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		attributes = append(attributes, otellog.String(key, record.Attributes[key]))
	}
	if record.Stream != "" {
		if _, exists := record.Attributes["log.iostream"]; !exists {
			attributes = append(attributes, otellog.String("log.iostream", string(record.Stream)))
		}
	}
	output.AddAttributes(attributes...)
	s.logger.Emit(ctx, output)
	return nil
}

func (s *OTLPSink) ForceFlush(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.provider.ForceFlush(ctx)
}

func (s *OTLPSink) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.provider.Shutdown(ctx)
}
