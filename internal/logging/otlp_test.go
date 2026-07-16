package logging

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectorlog "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

func TestNewOTLPSinkDisabledWithoutEndpoint(t *testing.T) {
	sink, err := NewOTLPSink(context.Background(), OTLPConfig{})
	if err != nil || sink != nil {
		t.Fatalf("NewOTLPSink() = (%v, %v), want (nil, nil)", sink, err)
	}
}

func TestOTLPSinkExportsIdentityAndLogAttributes(t *testing.T) {
	requests := make(chan *collectorlog.ExportLogsServiceRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/logs" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		var decoded collectorlog.ExportLogsServiceRequest
		if err := proto.Unmarshal(body, &decoded); err != nil {
			t.Errorf("decode OTLP request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- &decoded
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink, err := NewOTLPSink(context.Background(), OTLPConfig{
		Endpoint:          server.URL + "/v1/logs",
		Protocol:          "http/protobuf",
		Insecure:          true,
		ServiceName:       "montainer",
		ServiceInstanceID: "acceptance-server",
		ServiceVersion:    "2.0.0",
		ResourceAttributes: map[string]string{
			"deployment.environment.name": "test",
		},
		QueueSize:      16,
		BatchSize:      8,
		ExportInterval: time.Hour,
		RequestTimeout: time.Second,
		ExportTimeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink() error = %v", err)
	}
	record := NewRecord(time.Now(), StreamStderr, "Bedrock warning")
	record.Attributes["minecraft.server.state"] = "running"
	if err := sink.Write(context.Background(), record); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sink.ForceFlush(ctx); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}

	select {
	case request := <-requests:
		if len(request.ResourceLogs) != 1 {
			t.Fatalf("ResourceLogs count = %d", len(request.ResourceLogs))
		}
		resourceAttrs := keyValues(request.ResourceLogs[0].Resource.Attributes)
		for key, want := range map[string]string{
			"service.name":                "montainer",
			"service.instance.id":         "acceptance-server",
			"service.version":             "2.0.0",
			"deployment.environment.name": "test",
		} {
			if resourceAttrs[key] != want {
				t.Errorf("resource attribute %s = %q, want %q", key, resourceAttrs[key], want)
			}
		}
		records := request.ResourceLogs[0].ScopeLogs[0].LogRecords
		if len(records) != 1 || records[0].Body.GetStringValue() != "Bedrock warning" {
			t.Fatalf("exported records = %+v", records)
		}
		logAttrs := keyValues(records[0].Attributes)
		if logAttrs["log.iostream"] != "stderr" || logAttrs["minecraft.server.state"] != "running" {
			t.Fatalf("log attributes = %#v", logAttrs)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for OTLP request")
	}
	if err := sink.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func keyValues(values []*common.KeyValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.Key] = value.Value.GetStringValue()
	}
	return result
}
