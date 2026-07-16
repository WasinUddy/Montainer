// Package api exposes Montainer's compatibility HTTP and WebSocket contract
// through Gin. Process, backup, and telemetry behavior remain outside Gin.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wasinuddy/montainer/v2/internal/backup"
	"github.com/wasinuddy/montainer/v2/internal/bedrock"
	"github.com/wasinuddy/montainer/v2/internal/config"
	logstream "github.com/wasinuddy/montainer/v2/internal/logging"
)

type Dependencies struct {
	Config           config.Config
	Supervisor       *bedrock.Supervisor
	Logs             *logstream.Hub
	Backup           *backup.Service
	StaticDir        string
	LifecycleContext context.Context
}

type Server struct {
	cfg        config.Config
	supervisor *bedrock.Supervisor
	logs       *logstream.Hub
	backup     *backup.Service
	staticDir  string
	lifecycle  context.Context
}

func NewRouter(dependencies Dependencies) (*gin.Engine, error) {
	if dependencies.Supervisor == nil {
		return nil, fmt.Errorf("API supervisor is required")
	}
	if dependencies.Logs == nil {
		return nil, fmt.Errorf("API log hub is required")
	}

	lifecycle := dependencies.LifecycleContext
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	server := &Server{
		cfg:        dependencies.Config,
		supervisor: dependencies.Supervisor,
		logs:       dependencies.Logs,
		backup:     dependencies.Backup,
		staticDir:  dependencies.StaticDir,
		lifecycle:  lifecycle,
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), securityHeaders())
	router.GET("/healthz", server.health)
	router.GET("/readyz", server.ready)

	base := strings.TrimSuffix(dependencies.Config.SubpathURL, "/")
	if base == "/" {
		base = ""
	}
	routes := router.Group(base)
	server.register(routes)
	if base != "" {
		routes.GET("/healthz", server.health)
		routes.GET("/readyz", server.ready)
	}
	return router, nil
}

func (s *Server) register(routes *gin.RouterGroup) {
	routes.GET("/", s.index)
	if strings.TrimSpace(s.staticDir) != "" {
		routes.StaticFS("/assets", http.Dir(filepath.Join(s.staticDir, "assets")))
	}
	routes.POST("/start", s.start)
	routes.GET("/status", s.status)
	routes.POST("/stop", s.stop)
	routes.POST("/toggle", s.toggle)
	routes.POST("/restart", s.restart)
	routes.POST("/command", s.command)
	routes.GET("/logs", s.getLogs)
	routes.GET("/instance_name", s.instanceName)
	routes.POST("/save", s.save)
	routes.GET("/ws/stream", s.stream)
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

func (s *Server) operationContext() (context.Context, context.CancelFunc) {
	// Restart is the longest control operation: stop timeout, post-stop sync,
	// then a separate pre-start sync.
	timeout := s.cfg.ShutdownTimeout + 2*s.cfg.LifecycleTimeout + 5*time.Second
	return context.WithTimeout(s.lifecycle, timeout)
}

func (s *Server) start(c *gin.Context) {
	ctx, cancel := s.operationContext()
	defer cancel()
	if err := s.supervisor.Start(ctx); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Server started successfully."})
}

func (s *Server) stop(c *gin.Context) {
	ctx, cancel := s.operationContext()
	defer cancel()
	if err := s.supervisor.Stop(ctx); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Server stopped successfully."})
}

func (s *Server) toggle(c *gin.Context) {
	ctx, cancel := s.operationContext()
	defer cancel()
	err := s.supervisor.WithExclusive(ctx, func(server *bedrock.Exclusive) error {
		snapshot, err := server.Snapshot()
		if err != nil {
			return err
		}
		if snapshot.IsRunning() {
			return server.Stop(ctx)
		}
		return server.Start(ctx)
	})
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Server state changed successfully."})
}

func (s *Server) restart(c *gin.Context) {
	ctx, cancel := s.operationContext()
	defer cancel()
	if err := s.supervisor.Restart(ctx); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Server restarted successfully."})
}

type commandRequest struct {
	Command string `json:"command" binding:"required"`
}

func (s *Server) command(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var request commandRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "A non-empty command is required."})
		return
	}
	if len(request.Command) > 4096 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Command is too long."})
		return
	}
	ctx, cancel := s.operationContext()
	defer cancel()
	if err := s.supervisor.SendCommand(ctx, request.Command); err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Command sent successfully."})
}

func (s *Server) status(c *gin.Context) {
	snapshot := s.supervisor.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"status":     "success",
		"is_running": snapshot.IsRunning(),
		"state":      snapshot.State,
		"pid":        snapshot.PID,
		"generation": snapshot.Generation,
		"started_at": snapshot.StartedAt,
		"stopped_at": snapshot.StoppedAt,
		"exit_code":  snapshot.ExitCode,
		"last_error": snapshot.LastError,
		"dropped_logs": gin.H{
			"file": s.logs.Dropped("file"),
			"otlp": s.logs.Dropped("otlp"),
		},
	})
}

func (s *Server) getLogs(c *gin.Context) {
	maxLines := 31
	if raw := c.Query("max_lines"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > s.cfg.LogHistorySize {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": fmt.Sprintf("max_lines must be between 1 and %d", s.cfg.LogHistorySize)})
			return
		}
		maxLines = parsed
	}
	records := s.logs.Recent(maxLines)
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, record.Body)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "logs": lines})
}

func (s *Server) instanceName(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance_name": s.cfg.InstanceName})
}

func (s *Server) save(c *gin.Context) {
	if s.backup == nil || !s.backup.Configured() {
		s.writeError(c, backup.ErrNotConfigured)
		return
	}
	// A disconnected browser must not interrupt a stopped world snapshot.
	// Application shutdown still cancels the operation, and recovery uses its
	// own bounded context to restore the prior server state.
	ctx, cancel := context.WithTimeout(s.lifecycle, s.cfg.BackupTimeout)
	defer cancel()
	result, err := s.backup.Save(ctx)
	if err != nil {
		s.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Data saved successfully.", "backup": result})
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	snapshot := s.supervisor.Snapshot()
	c.JSON(http.StatusOK, gin.H{"status": "ok", "server_state": snapshot.State})
}

func (s *Server) index(c *gin.Context) {
	if strings.TrimSpace(s.staticDir) == "" {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Web UI is not configured."})
		return
	}
	index := filepath.Join(s.staticDir, "index.html")
	if _, err := filepath.Abs(index); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Web UI path is invalid."})
		return
	}
	c.File(index)
}

func (s *Server) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, bedrock.ErrAlreadyRunning), errors.Is(err, bedrock.ErrNotRunning), errors.Is(err, backup.ErrInProgress):
		status = http.StatusConflict
	case errors.Is(err, bedrock.ErrInvalidCommand):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, backup.ErrNotConfigured):
		status = http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		status = http.StatusRequestTimeout
	}
	c.JSON(status, gin.H{"detail": gin.H{"status": "error", "message": err.Error()}})
}
