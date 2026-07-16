package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type streamMessage struct {
	Logs      []string `json:"logs"`
	IsRunning bool     `json:"is_running"`
	State     string   `json:"state"`
}

const consoleHistoryLines = 31

func (s *Server) stream(c *gin.Context) {
	connection, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "stream closed")
	ctx := connection.CloseRead(c.Request.Context())

	// The first frame below is the history snapshot. A one-element live queue
	// acts as an invalidation signal: because each update is another complete
	// snapshot, coalescing bursts loses no visible state and prevents replay
	// amplification for newly connected clients.
	subscription, err := s.logs.Subscribe(1, 0)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "log stream unavailable")
		return
	}
	defer subscription.Close()

	if err := s.writeStreamMessage(ctx, connection); err != nil {
		return
	}
	lastState := s.supervisor.Snapshot()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-subscription.Records:
			if !ok || s.writeStreamMessage(ctx, connection) != nil {
				return
			}
		case <-ticker.C:
			snapshot := s.supervisor.Snapshot()
			if snapshot.State != lastState.State || snapshot.Generation != lastState.Generation {
				lastState = snapshot
				if s.writeStreamMessage(ctx, connection) != nil {
					return
				}
			}
		}
	}
}

func (s *Server) writeStreamMessage(ctx context.Context, connection *websocket.Conn) error {
	records := s.logs.Recent(consoleHistoryLines)
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, record.Body)
	}
	snapshot := s.supervisor.Snapshot()
	payload, err := json.Marshal(streamMessage{
		Logs:      lines,
		IsRunning: snapshot.IsRunning(),
		State:     string(snapshot.State),
	})
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return connection.Write(writeCtx, websocket.MessageText, payload)
}
