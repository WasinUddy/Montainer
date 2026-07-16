package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wasinuddy/montainer/v2/internal/api"
	"github.com/wasinuddy/montainer/v2/internal/backup"
	"github.com/wasinuddy/montainer/v2/internal/bedrock"
	"github.com/wasinuddy/montainer/v2/internal/config"
	logstream "github.com/wasinuddy/montainer/v2/internal/logging"
	"github.com/wasinuddy/montainer/v2/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Printf("Montainer stopped with an error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	hub := logstream.NewHub(logstream.Options{
		HistorySize:   cfg.LogHistorySize,
		SinkQueueSize: cfg.LogSinkQueueSize,
	})
	fileSink, err := logstream.NewFileSinkWithOptions(
		filepath.Join(cfg.LogDir, "instance.log"),
		logstream.FileSinkOptions{
			MaxBytes:   cfg.LogFileMaxBytes,
			MaxBackups: cfg.LogFileMaxBackups,
		},
	)
	if err != nil {
		return fmt.Errorf("configure local log: %w", err)
	}
	if err := hub.AddSink("file", fileSink, cfg.LogSinkQueueSize); err != nil {
		return err
	}

	if cfg.OTel.Enabled() {
		endpoint, err := cfg.OTel.ResolvedLogsEndpoint()
		if err != nil {
			return err
		}
		otlpSink, err := logstream.NewOTLPSink(context.Background(), logstream.OTLPConfig{
			Endpoint:           endpoint,
			Protocol:           cfg.OTel.Protocol,
			Insecure:           cfg.OTel.Insecure,
			ServiceName:        cfg.OTel.ServiceName,
			ServiceInstanceID:  cfg.InstanceName,
			ServiceVersion:     cfg.OTel.ServiceVersion,
			ResourceAttributes: cfg.OTel.ResourceAttributes,
			QueueSize:          cfg.OTel.QueueSize,
			BatchSize:          cfg.OTel.BatchSize,
			ExportInterval:     cfg.OTel.ExportInterval,
			RequestTimeout:     cfg.OTel.RequestTimeout,
			ExportTimeout:      cfg.OTel.ExportTimeout,
		})
		if err != nil {
			return fmt.Errorf("configure OTLP logs: %w", err)
		}
		if err := hub.AddSink("otlp", otlpSink, cfg.LogSinkQueueSize); err != nil {
			return err
		}
		log.Printf("optional OTLP log export enabled for %s", endpoint)
	}

	filesystem, err := bedrock.NewFilesystemLifecycle(bedrock.FilesystemConfig{
		InstanceDir:      cfg.InstanceDir,
		ConfigDir:        cfg.ConfigDir,
		ResourcePacksDir: cfg.ResourcePacksDir,
	})
	if err != nil {
		return fmt.Errorf("configure Bedrock filesystem: %w", err)
	}
	supervisor, err := bedrock.NewSupervisor(bedrock.SupervisorConfig{
		Executable:       cfg.BedrockServerPath,
		WorkingDir:       cfg.InstanceDir,
		ShutdownTimeout:  cfg.ShutdownTimeout,
		LifecycleTimeout: cfg.LifecycleTimeout,
		Publisher:        hub,
		Lifecycle:        filesystem,
	})
	if err != nil {
		return fmt.Errorf("configure Bedrock supervisor: %w", err)
	}

	var objectStore storage.ObjectStore
	if cfg.S3.Enabled() {
		objectStore, err = storage.NewS3Store(context.Background(), storage.S3Config{
			Endpoint:        cfg.S3.Endpoint,
			AccessKeyID:     cfg.S3.KeyID,
			SecretAccessKey: cfg.S3.SecretKey,
			Bucket:          cfg.S3.Bucket,
			Region:          cfg.S3.Region,
			UsePathStyle:    cfg.S3.Endpoint != "",
		})
		if err != nil {
			return fmt.Errorf("configure backup storage: %w", err)
		}
	}
	backupService, err := backup.NewService(backup.Options{
		Supervisor:      supervisor,
		Store:           objectStore,
		Paths:           backup.Paths{InstanceDir: cfg.InstanceDir, ConfigDir: cfg.ConfigDir},
		InstanceName:    cfg.InstanceName,
		RecoveryTimeout: cfg.LifecycleTimeout,
	})
	if err != nil {
		return fmt.Errorf("configure backup service: %w", err)
	}

	staticDir := strings.TrimSpace(os.Getenv("STATIC_DIR"))
	if staticDir == "" {
		staticDir = "./web/dist"
	}
	router, err := api.NewRouter(api.Dependencies{
		Config:           cfg,
		Supervisor:       supervisor,
		Logs:             hub,
		Backup:           backupService,
		StaticDir:        staticDir,
		LifecycleContext: appCtx,
	})
	if err != nil {
		return fmt.Errorf("configure HTTP API: %w", err)
	}

	if cfg.BedrockAutoStart {
		startupCtx, cancel := context.WithTimeout(appCtx, cfg.LifecycleTimeout)
		err = supervisor.Start(startupCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("auto-start Bedrock server: %w", err)
		}
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serveErrors := make(chan error, 1)
	go func() {
		log.Printf("Montainer v2 listening on %s", cfg.ListenAddr)
		serveErrors <- httpServer.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-appCtx.Done():
	case serveErr = <-serveErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = fmt.Errorf("serve HTTP: %w", serveErr)
		} else {
			serveErr = nil
		}
	}

	// Cancel server-owned handler work even when the listener itself failed.
	// HTTP draining runs concurrently so it cannot consume the Bedrock stop
	// budget before the supervisor gets a chance to send a graceful stop.
	stopSignals()
	// A canceled backup may still finish stop/post-stop sync, restore its prior
	// running state, and then yield to final shutdown. Budget for both stops and
	// all three possible filesystem lifecycle phases.
	managementBudget := 2*cfg.ShutdownTimeout + 3*cfg.LifecycleTimeout + 5*time.Second
	httpShutdownCtx, cancelHTTPShutdown := context.WithTimeout(context.Background(), managementBudget)
	defer cancelHTTPShutdown()
	httpShutdown := make(chan error, 1)
	go func() {
		httpShutdown <- httpServer.Shutdown(httpShutdownCtx)
	}()

	bedrockShutdownCtx, cancelBedrockShutdown := context.WithTimeout(context.Background(), managementBudget)
	bedrockErr := supervisor.Shutdown(bedrockShutdownCtx)
	cancelBedrockShutdown()
	httpErr := <-httpShutdown

	// Export is optional. Bound best-effort draining so a failed Collector can
	// neither hang shutdown nor turn an otherwise clean exit into a failure.
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	flushErr := hub.ForceFlush(flushCtx)
	cancelFlush()
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	closeErr := hub.Shutdown(closeCtx)
	cancelClose()
	if telemetryErr := errors.Join(flushErr, closeErr); telemetryErr != nil {
		log.Printf("log sinks did not fully flush during shutdown: %v", telemetryErr)
	}

	return errors.Join(serveErr, httpErr, bedrockErr)
}
