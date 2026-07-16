package logging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type sinkFunc func(context.Context, Record) error

func (f sinkFunc) Write(ctx context.Context, record Record) error { return f(ctx, record) }

func TestHubRetainsReplaysAndClonesRecords(t *testing.T) {
	hub := NewHub(Options{HistorySize: 2, SinkQueueSize: 4})
	attributes := map[string]string{"key": "original"}
	hub.Publish(Record{Body: "one", Stream: StreamStdout, Attributes: attributes})
	attributes["key"] = "mutated-after-publish"
	hub.Publish(NewRecord(time.Now(), StreamStderr, "two"))
	hub.Publish(NewRecord(time.Now(), StreamStdout, "three"))

	recent := hub.Recent(10)
	if len(recent) != 2 || recent[0].Body != "two" || recent[1].Body != "three" {
		t.Fatalf("Recent() = %+v", recent)
	}
	recent[0].Attributes["changed"] = "yes"
	if _, changed := hub.Recent(1)[0].Attributes["changed"]; changed {
		t.Fatal("Recent returned a mutable reference to hub history")
	}

	subscription, err := hub.Subscribe(1, 2)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer subscription.Close()
	for _, want := range []string{"two", "three"} {
		select {
		case record := <-subscription.Records:
			if record.Body != want {
				t.Fatalf("replayed body = %q, want %q", record.Body, want)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for replay")
		}
	}
}

func TestSlowSinkDoesNotBlockOtherSinks(t *testing.T) {
	hub := NewHub(Options{HistorySize: 32, SinkQueueSize: 32})
	blocked := make(chan struct{})
	remoteStarted := make(chan struct{}, 1)
	remote := sinkFunc(func(context.Context, Record) error {
		select {
		case remoteStarted <- struct{}{}:
		default:
		}
		<-blocked
		return errors.New("collector unavailable")
	})
	localRecords := make(chan Record, 32)
	local := sinkFunc(func(_ context.Context, record Record) error {
		localRecords <- record
		return nil
	})
	if err := hub.AddSink("remote", remote, 1); err != nil {
		t.Fatal(err)
	}
	if err := hub.AddSink("local", local, 32); err != nil {
		t.Fatal(err)
	}

	hub.Publish(NewRecord(time.Now(), StreamStdout, "first"))
	select {
	case <-remoteStarted:
	case <-time.After(time.Second):
		t.Fatal("remote worker did not start")
	}
	for index := 0; index < 10; index++ {
		hub.Publish(NewRecord(time.Now(), StreamStdout, "local"))
	}
	for index := 0; index < 11; index++ {
		select {
		case <-localRecords:
		case <-time.After(time.Second):
			t.Fatalf("local sink received only %d records", index)
		}
	}
	if hub.Dropped("remote") == 0 {
		t.Fatal("remote dropped count = 0, want queue overflow")
	}

	close(blocked)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if hub.LastError("remote") == nil {
		t.Fatal("LastError(remote) = nil")
	}
}

func TestSinkWriteFailureCountsAsDropped(t *testing.T) {
	hub := NewHub(Options{HistorySize: 1, SinkQueueSize: 1})
	if err := hub.AddSink("failed", sinkFunc(func(context.Context, Record) error {
		return errors.New("disk full")
	}), 1); err != nil {
		t.Fatal(err)
	}
	hub.Publish(NewRecord(time.Now(), StreamStdout, "not persisted"))

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && hub.LastError("failed") == nil {
		time.Sleep(time.Millisecond)
	}
	if hub.LastError("failed") == nil {
		t.Fatal("sink failure was not recorded")
	}
	if got := hub.Dropped("failed"); got != 1 {
		t.Fatalf("Dropped(failed) = %d, want 1", got)
	}
	if err := hub.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFileSinkAppendsPlainLogFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "instance.log")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	if err := sink.Write(context.Background(), NewRecord(time.Now(), StreamStdout, "first\n")); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(context.Background(), NewRecord(time.Now(), StreamStderr, "second")); err != nil {
		t.Fatal(err)
	}
	if err := sink.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "first\nsecond\n" {
		t.Fatalf("contents = %q", contents)
	}
}

func TestFileSinkRotatesAndBoundsBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.log")
	sink, err := NewFileSinkWithOptions(path, FileSinkOptions{MaxBytes: 12, MaxBackups: 2})
	if err != nil {
		t.Fatalf("NewFileSinkWithOptions() error = %v", err)
	}
	for _, body := range []string{"first", "second", "third"} {
		if err := sink.Write(context.Background(), NewRecord(time.Now(), StreamStdout, body)); err != nil {
			t.Fatalf("Write(%q) error = %v", body, err)
		}
	}
	if err := sink.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		path:                    "third\n",
		rotatedLogPath(path, 1): "second\n",
		rotatedLogPath(path, 2): "first\n",
	} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if string(contents) != want {
			t.Fatalf("contents of %s = %q, want %q", path, contents, want)
		}
	}
	if _, err := os.Stat(rotatedLogPath(path, 3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("third backup exists or stat failed unexpectedly: %v", err)
	}
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	hub := NewHub(Options{HistorySize: 128})
	var wait sync.WaitGroup
	for writer := 0; writer < 8; writer++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < 100; index++ {
				hub.Publish(NewRecord(time.Now(), StreamStdout, "line"))
			}
		}()
	}
	for reader := 0; reader < 8; reader++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < 20; index++ {
				subscription, err := hub.Subscribe(8, 4)
				if err != nil {
					t.Errorf("Subscribe() error = %v", err)
					return
				}
				subscription.Close()
				_ = hub.Recent(31)
			}
		}()
	}
	wait.Wait()
}
