package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/wasinuddy/montainer/v2/internal/bedrock"
	"github.com/wasinuddy/montainer/v2/internal/config"
	logstream "github.com/wasinuddy/montainer/v2/internal/logging"
)

func TestWebSocketSendsOneBoundedSnapshotThenLiveUpdates(t *testing.T) {
	t.Parallel()

	hub := logstream.NewHub(logstream.Options{HistorySize: 1_000})
	for index := 0; index < 100; index++ {
		hub.Publish(logstream.NewRecord(time.Now(), logstream.StreamStdout, strings.Repeat("x", index+1)))
	}
	supervisor, err := bedrock.NewSupervisor(bedrock.SupervisorConfig{
		Executable: "/not-started-in-this-test",
		WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	router, err := NewRouter(Dependencies{
		Config: config.Config{
			SubpathURL:       "/",
			LogHistorySize:   1_000,
			ShutdownTimeout:  time.Second,
			LifecycleTimeout: time.Second,
		},
		Supervisor: supervisor,
		Logs:       hub,
	})
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	server := httptest.NewServer(router)
	defer server.Close()

	connection, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/stream", nil)
	if err != nil {
		t.Fatalf("websocket.Dial() error = %v", err)
	}
	defer connection.CloseNow()
	eventuallySubscriberCount(t, hub, 1)

	first := readTestStreamMessage(t, connection)
	if len(first.Logs) != consoleHistoryLines {
		t.Fatalf("initial log count = %d, want %d", len(first.Logs), consoleHistoryLines)
	}

	next := make(chan streamMessage, 1)
	readErrors := make(chan error, 1)
	go func() {
		_, payload, readErr := connection.Read(context.Background())
		if readErr != nil {
			readErrors <- readErr
			return
		}
		var message streamMessage
		if readErr = json.Unmarshal(payload, &message); readErr != nil {
			readErrors <- readErr
			return
		}
		next <- message
	}()

	select {
	case message := <-next:
		t.Fatalf("received duplicate replay frame with %d logs", len(message.Logs))
	case readErr := <-readErrors:
		t.Fatalf("read second stream frame: %v", readErr)
	case <-time.After(150 * time.Millisecond):
	}

	hub.Publish(logstream.NewRecord(time.Now(), logstream.StreamStdout, "live-token"))
	select {
	case message := <-next:
		if len(message.Logs) != consoleHistoryLines || message.Logs[len(message.Logs)-1] != "live-token" {
			t.Fatalf("unexpected live snapshot: %#v", message.Logs)
		}
	case readErr := <-readErrors:
		t.Fatalf("read live stream frame: %v", readErr)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for live stream frame")
	}

	if err := connection.Close(websocket.StatusNormalClosure, "test complete"); err != nil {
		t.Fatalf("close websocket: %v", err)
	}
	eventuallySubscriberCount(t, hub, 0)
}

func readTestStreamMessage(t *testing.T, connection *websocket.Conn) streamMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, payload, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("read stream frame: %v", err)
	}
	var message streamMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("decode stream frame: %v", err)
	}
	return message
}

func eventuallySubscriberCount(t *testing.T, hub *logstream.Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscriber count = %d, want %d", hub.SubscriberCount(), want)
}
