// Package config loads Montainer's runtime configuration from environment
// variables. It deliberately has no dependency on the HTTP transport so the
// same configuration can be used by the production binary and acceptance
// tests.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr       = ":8000"
	defaultInstanceDir      = "./instance"
	defaultConfigDir        = "./configs"
	defaultResourcePacksDir = "./resource_packs"
	defaultLogDir           = "."
	defaultBedrockPath      = "./bedrock_server"
	defaultInstanceName     = "Montainer"
	defaultServiceName      = "montainer"
)

// Config contains all process-level Montainer configuration.
type Config struct {
	ListenAddr        string
	SubpathURL        string
	InstanceName      string
	BedrockServerPath string
	InstanceDir       string
	ConfigDir         string
	ResourcePacksDir  string
	LogDir            string
	BedrockAutoStart  bool
	ShutdownTimeout   time.Duration
	LifecycleTimeout  time.Duration
	BackupTimeout     time.Duration
	LogHistorySize    int
	LogSinkQueueSize  int
	LogFileMaxBytes   int64
	LogFileMaxBackups int

	S3   S3Config
	OTel OTelConfig
}

// S3Config preserves the environment contract used by the Python backend.
type S3Config struct {
	Endpoint  string
	KeyID     string
	SecretKey string
	Bucket    string
	Region    string
}

// Enabled reports whether S3 backup support was requested. Detailed
// credential and bucket validation belongs to the storage adapter.
func (c S3Config) Enabled() bool { return strings.TrimSpace(c.Endpoint) != "" }

// OTelConfig contains optional OTLP log-export settings. A blank endpoint
// means telemetry is disabled and is a supported standalone mode.
type OTelConfig struct {
	SDKDisabled        bool
	Endpoint           string
	LogsEndpoint       string
	Protocol           string
	Insecure           bool
	ServiceName        string
	ServiceVersion     string
	ResourceAttributes map[string]string
	QueueSize          int
	BatchSize          int
	ExportInterval     time.Duration
	RequestTimeout     time.Duration
	ExportTimeout      time.Duration
}

// Enabled reports whether either standard OTLP endpoint variable was set.
func (c OTelConfig) Enabled() bool {
	return !c.SDKDisabled && (strings.TrimSpace(c.LogsEndpoint) != "" || strings.TrimSpace(c.Endpoint) != "")
}

// ResolvedLogsEndpoint returns the full HTTP endpoint for logs. Per the OTLP
// specification, a signal-specific endpoint is used as-is, while /v1/logs is
// appended to the generic endpoint (including after any existing path prefix).
func (c OTelConfig) ResolvedLogsEndpoint() (string, error) {
	raw := strings.TrimSpace(c.LogsEndpoint)
	signalSpecific := raw != ""
	if !signalSpecific {
		raw = strings.TrimSpace(c.Endpoint)
	}
	if raw == "" {
		return "", nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse OTLP logs endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("OTLP logs endpoint must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("OTLP logs endpoint must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("OTLP logs endpoint must not include a query or fragment")
	}
	if !signalSpecific {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1/logs"
	}
	return u.String(), nil
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads configuration through lookup. It is exported so tests and
// embedders can supply an isolated environment.
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("environment lookup must not be nil")
	}

	boolValue := func(key string, fallback bool) (bool, error) {
		raw, ok := lookup(key)
		if !ok || strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return false, fmt.Errorf("%s must be a boolean: %w", key, err)
		}
		return value, nil
	}
	durationValue := func(key string, fallback time.Duration) (time.Duration, error) {
		raw, ok := lookup(key)
		if !ok || strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive duration", key)
		}
		return value, nil
	}
	millisecondDurationValue := func(key string, fallback time.Duration) (time.Duration, error) {
		raw, ok := lookup(key)
		if !ok || strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		raw = strings.TrimSpace(raw)
		if milliseconds, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			if milliseconds <= 0 {
				return 0, fmt.Errorf("%s must be a positive duration", key)
			}
			return time.Duration(milliseconds) * time.Millisecond, nil
		}
		value, parseErr := time.ParseDuration(raw)
		if parseErr != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive duration in milliseconds", key)
		}
		return value, nil
	}
	intValue := func(key string, fallback int) (int, error) {
		raw, ok := lookup(key)
		if !ok || strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return value, nil
	}
	stringValue := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return fallback
	}
	firstString := func(primary, secondary, fallback string) string {
		if value, ok := lookup(primary); ok && strings.TrimSpace(value) != "" {
			return value
		}
		return stringValue(secondary, fallback)
	}

	autoStart, err := boolValue("BEDROCK_AUTO_START", true)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationValue("BEDROCK_SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	lifecycleTimeout, err := durationValue("BEDROCK_LIFECYCLE_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	backupTimeout, err := durationValue("BACKUP_TIMEOUT", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	historySize, err := intValue("LOG_HISTORY_SIZE", 1_000)
	if err != nil {
		return Config{}, err
	}
	sinkQueueSize, err := intValue("LOG_SINK_QUEUE_SIZE", 2_048)
	if err != nil {
		return Config{}, err
	}
	logFileMaxSizeMB, err := intValue("LOG_FILE_MAX_SIZE_MB", 100)
	if err != nil {
		return Config{}, err
	}
	if int64(logFileMaxSizeMB) > (1<<63-1)/(1024*1024) {
		return Config{}, fmt.Errorf("LOG_FILE_MAX_SIZE_MB is too large")
	}
	logFileMaxBackups, err := intValue("LOG_FILE_MAX_BACKUPS", 5)
	if err != nil {
		return Config{}, err
	}

	otlpInsecure, err := boolValue("OTEL_EXPORTER_OTLP_LOGS_INSECURE", false)
	if err != nil {
		return Config{}, err
	}
	if _, signalSet := lookup("OTEL_EXPORTER_OTLP_LOGS_INSECURE"); !signalSet {
		otlpInsecure, err = boolValue("OTEL_EXPORTER_OTLP_INSECURE", false)
		if err != nil {
			return Config{}, err
		}
	}
	otlpDisabled, err := boolValue("OTEL_SDK_DISABLED", false)
	if err != nil {
		return Config{}, err
	}
	otlpQueueSize, err := intValue("OTEL_BLRP_MAX_QUEUE_SIZE", 2_048)
	if err != nil {
		return Config{}, err
	}
	otlpBatchSize, err := intValue("OTEL_BLRP_MAX_EXPORT_BATCH_SIZE", 512)
	if err != nil {
		return Config{}, err
	}
	otlpInterval, err := millisecondDurationValue("OTEL_BLRP_SCHEDULE_DELAY", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	otlpTimeout, err := millisecondDurationValue("OTEL_BLRP_EXPORT_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	otlpRequestTimeout := 10 * time.Second
	if value, ok := lookup("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT"); ok && strings.TrimSpace(value) != "" {
		otlpRequestTimeout, err = millisecondDurationValue("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", 10*time.Second)
	} else {
		otlpRequestTimeout, err = millisecondDurationValue("OTEL_EXPORTER_OTLP_TIMEOUT", 10*time.Second)
	}
	if err != nil {
		return Config{}, err
	}

	resourceAttributes, err := parseAttributes(stringValue("OTEL_RESOURCE_ATTRIBUTES", ""))
	if err != nil {
		return Config{}, fmt.Errorf("OTEL_RESOURCE_ATTRIBUTES: %w", err)
	}

	cfg := Config{
		ListenAddr:        stringValue("LISTEN_ADDR", defaultListenAddr),
		SubpathURL:        normalizeSubpath(stringValue("SUBPATH_URL", "/")),
		InstanceName:      stringValue("INSTANCE_NAME", defaultInstanceName),
		BedrockServerPath: stringValue("BEDROCK_SERVER_PATH", defaultBedrockPath),
		InstanceDir:       stringValue("INSTANCE_DIR", defaultInstanceDir),
		ConfigDir:         stringValue("CONFIG_DIR", defaultConfigDir),
		ResourcePacksDir:  stringValue("RESOURCE_PACKS_DIR", defaultResourcePacksDir),
		LogDir:            stringValue("LOG_DIR", defaultLogDir),
		BedrockAutoStart:  autoStart,
		ShutdownTimeout:   shutdownTimeout,
		LifecycleTimeout:  lifecycleTimeout,
		BackupTimeout:     backupTimeout,
		LogHistorySize:    historySize,
		LogSinkQueueSize:  sinkQueueSize,
		LogFileMaxBytes:   int64(logFileMaxSizeMB) * 1024 * 1024,
		LogFileMaxBackups: logFileMaxBackups,
		S3: S3Config{
			Endpoint:  stringValue("AWS_S3_ENDPOINT", ""),
			KeyID:     stringValue("AWS_S3_KEY_ID", ""),
			SecretKey: stringValue("AWS_S3_SECRET_KEY", ""),
			Bucket:    stringValue("AWS_S3_BUCKET_NAME", ""),
			Region:    stringValue("AWS_S3_REGION", ""),
		},
		OTel: OTelConfig{
			SDKDisabled:        otlpDisabled,
			Endpoint:           stringValue("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			LogsEndpoint:       stringValue("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", ""),
			Protocol:           firstString("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"),
			Insecure:           otlpInsecure,
			ServiceName:        stringValue("OTEL_SERVICE_NAME", defaultServiceName),
			ServiceVersion:     stringValue("MONTAINER_VERSION", ""),
			ResourceAttributes: resourceAttributes,
			QueueSize:          otlpQueueSize,
			BatchSize:          otlpBatchSize,
			ExportInterval:     otlpInterval,
			RequestTimeout:     otlpRequestTimeout,
			ExportTimeout:      otlpTimeout,
		},
	}

	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return Config{}, fmt.Errorf("LISTEN_ADDR must not be blank")
	}
	if strings.TrimSpace(cfg.InstanceName) == "" {
		return Config{}, fmt.Errorf("INSTANCE_NAME must not be blank")
	}
	for key, value := range map[string]string{
		"BEDROCK_SERVER_PATH": cfg.BedrockServerPath,
		"INSTANCE_DIR":        cfg.InstanceDir,
		"CONFIG_DIR":          cfg.ConfigDir,
		"RESOURCE_PACKS_DIR":  cfg.ResourcePacksDir,
		"LOG_DIR":             cfg.LogDir,
	} {
		if strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("%s must not be blank", key)
		}
	}
	if cfg.OTel.BatchSize > cfg.OTel.QueueSize {
		return Config{}, fmt.Errorf("OTEL_BLRP_MAX_EXPORT_BATCH_SIZE must not exceed OTEL_BLRP_MAX_QUEUE_SIZE")
	}
	if cfg.OTel.Enabled() {
		if cfg.OTel.Protocol != "http/protobuf" {
			return Config{}, fmt.Errorf("OTLP log protocol %q is unsupported; use http/protobuf", cfg.OTel.Protocol)
		}
		if _, err := cfg.OTel.ResolvedLogsEndpoint(); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

func normalizeSubpath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return "/"
	}
	cleaned := path.Clean("/" + strings.Trim(value, "/"))
	return cleaned + "/"
}

func parseAttributes(raw string) (map[string]string, error) {
	attributes := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return attributes, nil
	}
	for _, item := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid key-value pair %q", item)
		}
		decoded, err := url.PathUnescape(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("decode attribute %q: %w", key, err)
		}
		attributes[strings.TrimSpace(key)] = decoded
	}
	return attributes, nil
}
