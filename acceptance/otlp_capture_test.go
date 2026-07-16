package acceptance_test

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"

	collectlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

type capturedLog struct {
	body               string
	severity           string
	resourceAttributes map[string]string
	attributes         map[string]string
}

type otlpCapture struct {
	server *httptest.Server

	mu       sync.Mutex
	logs     []capturedLog
	paths    []string
	errors   []string
	requests int
}

func newOTLPCapture() *otlpCapture {
	capture := &otlpCapture{}
	capture.server = httptest.NewServer(http.HandlerFunc(capture.handle))
	return capture
}

func (c *otlpCapture) endpoint() string {
	return c.server.URL
}

func (c *otlpCapture) close() {
	if c != nil && c.server != nil {
		c.server.Close()
	}
}

func (c *otlpCapture) handle(response http.ResponseWriter, request *http.Request) {
	c.mu.Lock()
	c.requests++
	c.paths = append(c.paths, request.URL.Path)
	c.mu.Unlock()

	if request.Method != http.MethodPost || request.URL.Path != "/v1/logs" {
		c.recordError(fmt.Sprintf("unexpected %s %s", request.Method, request.URL.Path))
		http.Error(response, "OTLP logs are accepted at POST /v1/logs", http.StatusNotFound)
		return
	}

	reader := io.Reader(request.Body)
	if strings.EqualFold(request.Header.Get("Content-Encoding"), "gzip") {
		compressed, err := gzip.NewReader(request.Body)
		if err != nil {
			c.recordError("open gzip body: " + err.Error())
			http.Error(response, "invalid gzip body", http.StatusBadRequest)
			return
		}
		defer compressed.Close()
		reader = compressed
	}
	body, err := io.ReadAll(io.LimitReader(reader, 8<<20))
	if err != nil {
		c.recordError("read request body: " + err.Error())
		http.Error(response, "cannot read request", http.StatusBadRequest)
		return
	}

	var exportRequest collectlogspb.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &exportRequest); err != nil {
		c.recordError("decode protobuf: " + err.Error())
		http.Error(response, "invalid OTLP protobuf", http.StatusBadRequest)
		return
	}
	c.capture(&exportRequest)

	encoded, err := proto.Marshal(&collectlogspb.ExportLogsServiceResponse{})
	if err != nil {
		c.recordError("encode response: " + err.Error())
		http.Error(response, "cannot encode response", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/x-protobuf")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(encoded)
}

func (c *otlpCapture) capture(request *collectlogspb.ExportLogsServiceRequest) {
	var captured []capturedLog
	for _, resourceLogs := range request.ResourceLogs {
		resourceAttributes := keyValues(resourceLogs.GetResource().GetAttributes())
		for _, scopeLogs := range resourceLogs.ScopeLogs {
			for _, record := range scopeLogs.LogRecords {
				captured = append(captured, capturedLog{
					body:               anyValue(record.Body),
					severity:           record.SeverityText,
					resourceAttributes: cloneMap(resourceAttributes),
					attributes:         keyValues(record.Attributes),
				})
			}
		}
	}
	c.mu.Lock()
	c.logs = append(c.logs, captured...)
	c.mu.Unlock()
}

func (c *otlpCapture) find(token string) (capturedLog, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, log := range c.logs {
		if strings.Contains(log.body, token) {
			return log, true
		}
	}
	return capturedLog{}, false
}

func (c *otlpCapture) recordError(message string) {
	c.mu.Lock()
	c.errors = append(c.errors, message)
	c.mu.Unlock()
}

func (c *otlpCapture) diagnostics() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	bodies := make([]string, 0, len(c.logs))
	for _, log := range c.logs {
		bodies = append(bodies, log.body)
	}
	return fmt.Sprintf("requests=%d paths=%v logs=%q errors=%v", c.requests, c.paths, bodies, c.errors)
}

func keyValues(values []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.Key] = anyValue(value.Value)
	}
	return result
}

func anyValue(value *commonpb.AnyValue) string {
	if value == nil {
		return ""
	}
	switch typed := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", typed.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", typed.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", typed.DoubleValue)
	case *commonpb.AnyValue_BytesValue:
		return fmt.Sprintf("%x", typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		items := make([]string, 0, len(typed.ArrayValue.Values))
		for _, item := range typed.ArrayValue.Values {
			items = append(items, anyValue(item))
		}
		return "[" + strings.Join(items, ",") + "]"
	case *commonpb.AnyValue_KvlistValue:
		items := make([]string, 0, len(typed.KvlistValue.Values))
		for key, item := range keyValues(typed.KvlistValue.Values) {
			items = append(items, key+"="+item)
		}
		sort.Strings(items)
		return "{" + strings.Join(items, ",") + "}"
	default:
		return ""
	}
}

func cloneMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
