package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadFromDefaultsPreserveStandaloneContract(t *testing.T) {
	cfg, err := LoadFrom(mapLookup(nil))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.ListenAddr != ":8000" || cfg.SubpathURL != "/" {
		t.Fatalf("unexpected HTTP defaults: %+v", cfg)
	}
	if cfg.BedrockServerPath != "./bedrock_server" || cfg.InstanceDir != "./instance" {
		t.Fatalf("unexpected Bedrock defaults: %+v", cfg)
	}
	if !cfg.BedrockAutoStart {
		t.Fatal("BedrockAutoStart = false, want true")
	}
	if cfg.BackupTimeout != 30*time.Minute {
		t.Fatalf("BackupTimeout = %v, want 30m", cfg.BackupTimeout)
	}
	if cfg.InstanceName != "Montainer" {
		t.Fatalf("InstanceName = %q", cfg.InstanceName)
	}
	if cfg.LogFileMaxBytes != 100*1024*1024 || cfg.LogFileMaxBackups != 5 {
		t.Fatalf("unexpected log rotation defaults: bytes=%d backups=%d", cfg.LogFileMaxBytes, cfg.LogFileMaxBackups)
	}
	if cfg.S3.Enabled() || cfg.OTel.Enabled() {
		t.Fatal("optional integrations must be disabled by default")
	}
}

func TestLoadFromPreservesExistingAndV2Environment(t *testing.T) {
	environment := map[string]string{
		"LISTEN_ADDR":         "127.0.0.1:9000",
		"SUBPATH_URL":         "servers/friends",
		"INSTANCE_NAME":       "friends",
		"BEDROCK_SERVER_PATH": "/fixtures/fakebedrock",
		"INSTANCE_DIR":        "/data/instance",
		"CONFIG_DIR":          "/data/configs",
		"RESOURCE_PACKS_DIR":  "/data/packs",
		"LOG_DIR":             "/data/logs",
		"BEDROCK_AUTO_START":  "false",
		"AWS_S3_ENDPOINT":     "http://minio:9000",
		"AWS_S3_KEY_ID":       "key",
		"AWS_S3_SECRET_KEY":   "secret",
		"AWS_S3_BUCKET_NAME":  "backups",
		"AWS_S3_REGION":       "us-east-1",
	}
	cfg, err := LoadFrom(mapLookup(environment))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.SubpathURL != "/servers/friends/" {
		t.Fatalf("SubpathURL = %q", cfg.SubpathURL)
	}
	if cfg.BedrockAutoStart {
		t.Fatal("BedrockAutoStart = true, want false")
	}
	if !cfg.S3.Enabled() || cfg.S3.Bucket != "backups" || cfg.S3.SecretKey != "secret" {
		t.Fatalf("S3 config was not preserved: %+v", cfg.S3)
	}
}

func TestOTelEndpointPrecedenceAndResolution(t *testing.T) {
	cfg, err := LoadFrom(mapLookup(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":      "http://collector:4318/prefix/",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "http://logs-collector:4318/custom/logs",
		"OTEL_SERVICE_NAME":                "custom-montainer",
		"OTEL_RESOURCE_ATTRIBUTES":         "deployment.environment.name=test,team=bedrock%20ops",
		"OTEL_BLRP_SCHEDULE_DELAY":         "250",
		"OTEL_BLRP_EXPORT_TIMEOUT":         "2s",
		"OTEL_EXPORTER_OTLP_TIMEOUT":       "9000",
		"OTEL_EXPORTER_OTLP_LOGS_TIMEOUT":  "750",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	endpoint, err := cfg.OTel.ResolvedLogsEndpoint()
	if err != nil {
		t.Fatalf("ResolvedLogsEndpoint() error = %v", err)
	}
	if endpoint != "http://logs-collector:4318/custom/logs" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if cfg.OTel.ExportInterval != 250*time.Millisecond || cfg.OTel.ExportTimeout != 2*time.Second {
		t.Fatalf("unexpected OTLP durations: %+v", cfg.OTel)
	}
	if cfg.OTel.RequestTimeout != 750*time.Millisecond {
		t.Fatalf("OTLP RequestTimeout = %v", cfg.OTel.RequestTimeout)
	}
	wantAttributes := map[string]string{"deployment.environment.name": "test", "team": "bedrock ops"}
	if !reflect.DeepEqual(cfg.OTel.ResourceAttributes, wantAttributes) {
		t.Fatalf("attributes = %#v, want %#v", cfg.OTel.ResourceAttributes, wantAttributes)
	}

	cfg.OTel.LogsEndpoint = ""
	endpoint, err = cfg.OTel.ResolvedLogsEndpoint()
	if err != nil {
		t.Fatalf("ResolvedLogsEndpoint() generic error = %v", err)
	}
	if endpoint != "http://collector:4318/prefix/v1/logs" {
		t.Fatalf("generic endpoint = %q", endpoint)
	}
}

func TestOTelSDKDisabledOverridesEndpoint(t *testing.T) {
	cfg, err := LoadFrom(mapLookup(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"OTEL_SDK_DISABLED":           "true",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.OTel.Enabled() {
		t.Fatal("OTel.Enabled() = true when OTEL_SDK_DISABLED=true")
	}
}

func TestLoadFromRejectsInvalidValues(t *testing.T) {
	tests := []map[string]string{
		{"BEDROCK_AUTO_START": "sometimes"},
		{"BACKUP_TIMEOUT": "0s"},
		{"LOG_HISTORY_SIZE": "0"},
		{"LOG_FILE_MAX_SIZE_MB": "0"},
		{"LOG_FILE_MAX_BACKUPS": "0"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT": "collector:4318"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318", "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"},
		{"OTEL_RESOURCE_ATTRIBUTES": "missing-value"},
		{"OTEL_BLRP_MAX_QUEUE_SIZE": "10", "OTEL_BLRP_MAX_EXPORT_BATCH_SIZE": "11"},
	}
	for _, environment := range tests {
		t.Run(strings.Join(sortedKeys(environment), "+"), func(t *testing.T) {
			if _, err := LoadFrom(mapLookup(environment)); err == nil {
				t.Fatalf("LoadFrom(%v) succeeded, want error", environment)
			}
		})
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
