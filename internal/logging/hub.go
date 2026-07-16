package logging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var ErrHubClosed = errors.New("log hub is closed")

// Options controls in-memory log retention and default per-sink queue size.
type Options struct {
	HistorySize   int
	SinkQueueSize int
}

// Hub retains recent records, broadcasts them to UI subscribers, and fans
// them out to isolated sink workers.
type Hub struct {
	mu sync.RWMutex

	history      []Record
	historySize  int
	defaultQueue int
	subscribers  map[uint64]chan Record
	nextSubID    uint64
	workers      map[string]*sinkWorker
	closed       bool
}

type sinkWorker struct {
	name    string
	sink    Sink
	queue   chan Record
	done    chan struct{}
	dropped atomic.Uint64
	pending atomic.Int64

	errMu   sync.RWMutex
	lastErr error
}

// NewHub creates a Hub. Non-positive option values use safe defaults.
func NewHub(options Options) *Hub {
	if options.HistorySize <= 0 {
		options.HistorySize = 1_000
	}
	if options.SinkQueueSize <= 0 {
		options.SinkQueueSize = 2_048
	}
	return &Hub{
		history:      make([]Record, 0, options.HistorySize),
		historySize:  options.HistorySize,
		defaultQueue: options.SinkQueueSize,
		subscribers:  make(map[uint64]chan Record),
		workers:      make(map[string]*sinkWorker),
	}
}

// AddSink attaches a named, independently queued sink.
func (h *Hub) AddSink(name string, sink Sink, queueSize int) error {
	if name == "" {
		return fmt.Errorf("sink name must not be blank")
	}
	if sink == nil {
		return fmt.Errorf("sink %q must not be nil", name)
	}
	if queueSize <= 0 {
		queueSize = h.defaultQueue
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrHubClosed
	}
	if _, exists := h.workers[name]; exists {
		return fmt.Errorf("sink %q is already registered", name)
	}
	worker := &sinkWorker{
		name:  name,
		sink:  sink,
		queue: make(chan Record, queueSize),
		done:  make(chan struct{}),
	}
	h.workers[name] = worker
	go worker.run()
	return nil
}

// Publish stores and distributes a record without waiting for any external
// exporter. If an individual queue is full, only that destination's copy is
// dropped.
func (h *Hub) Publish(record Record) {
	now := time.Now().UTC()
	if record.Timestamp.IsZero() {
		record.Timestamp = now
	}
	if record.ObservedTimestamp.IsZero() {
		record.ObservedTimestamp = now
	}
	record = record.clone()
	if record.Attributes == nil {
		record.Attributes = make(map[string]string)
	}
	if _, ok := record.Attributes["log.iostream"]; !ok && record.Stream != "" {
		record.Attributes["log.iostream"] = string(record.Stream)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if len(h.history) == h.historySize {
		copy(h.history, h.history[1:])
		h.history[len(h.history)-1] = record.clone()
	} else {
		h.history = append(h.history, record.clone())
	}
	for _, subscriber := range h.subscribers {
		select {
		case subscriber <- record.clone():
		default:
		}
	}
	for _, worker := range h.workers {
		worker.pending.Add(1)
		select {
		case worker.queue <- record.clone():
		default:
			worker.pending.Add(-1)
			worker.dropped.Add(1)
		}
	}
}

// Recent returns up to limit records ordered oldest to newest.
func (h *Hub) Recent(limit int) []Record {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}
	start := len(h.history) - limit
	records := make([]Record, 0, limit)
	for _, record := range h.history[start:] {
		records = append(records, record.clone())
	}
	return records
}

// Subscription is a replay-plus-live stream. Close is idempotent.
type Subscription struct {
	Records <-chan Record
	once    sync.Once
	cancel  func()
}

// Close removes the subscriber and closes its Records channel.
func (s *Subscription) Close() {
	if s == nil || s.cancel == nil {
		return
	}
	s.once.Do(s.cancel)
}

// Subscribe atomically replays recent records and then provides live records,
// avoiding the gap between a separate Recent call and subscription.
func (h *Hub) Subscribe(buffer, replay int) (*Subscription, error) {
	if buffer <= 0 {
		buffer = 1
	}
	if replay < 0 {
		replay = 0
	}
	if buffer < replay {
		buffer = replay
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrHubClosed
	}
	id := h.nextSubID
	h.nextSubID++
	channel := make(chan Record, buffer)
	if replay > len(h.history) {
		replay = len(h.history)
	}
	for _, record := range h.history[len(h.history)-replay:] {
		channel <- record.clone()
	}
	h.subscribers[id] = channel
	return &Subscription{
		Records: channel,
		cancel: func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if existing, ok := h.subscribers[id]; ok {
				delete(h.subscribers, id)
				close(existing)
			}
		},
	}, nil
}

// Dropped returns the number of records omitted because a sink queue was full
// or the sink rejected a write (for example, when the log volume is full).
func (h *Hub) Dropped(name string) uint64 {
	h.mu.RLock()
	worker := h.workers[name]
	h.mu.RUnlock()
	if worker == nil {
		return 0
	}
	return worker.dropped.Load()
}

// SubscriberCount reports active live-stream subscriptions. It is primarily
// useful for health diagnostics and leak regression tests.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// LastError returns the most recent error reported by a sink worker.
func (h *Hub) LastError(name string) error {
	h.mu.RLock()
	worker := h.workers[name]
	h.mu.RUnlock()
	if worker == nil {
		return nil
	}
	worker.errMu.RLock()
	defer worker.errMu.RUnlock()
	return worker.lastErr
}

// ForceFlush asks all flush-capable sinks to export buffered records.
func (h *Hub) ForceFlush(ctx context.Context) error {
	h.mu.RLock()
	workers := make([]*sinkWorker, 0, len(h.workers))
	for _, worker := range h.workers {
		workers = append(workers, worker)
	}
	h.mu.RUnlock()

	var errs []error
	for _, worker := range workers {
		if err := worker.waitDrained(ctx); err != nil {
			errs = append(errs, fmt.Errorf("drain sink %q: %w", worker.name, err))
			continue
		}
		if flusher, ok := worker.sink.(Flusher); ok {
			if err := flusher.ForceFlush(ctx); err != nil {
				errs = append(errs, fmt.Errorf("flush sink %q: %w", worker.name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Shutdown stops accepting records, drains each independent queue, and closes
// sinks. The caller's context bounds the best-effort drain.
func (h *Hub) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		for id, subscriber := range h.subscribers {
			delete(h.subscribers, id)
			close(subscriber)
		}
		for _, worker := range h.workers {
			close(worker.queue)
		}
	}
	workers := make([]*sinkWorker, 0, len(h.workers))
	for _, worker := range h.workers {
		workers = append(workers, worker)
	}
	h.mu.Unlock()

	var errs []error
	for _, worker := range workers {
		select {
		case <-worker.done:
		case <-ctx.Done():
			return errors.Join(append(errs, ctx.Err())...)
		}
	}
	for _, worker := range workers {
		if shutdowner, ok := worker.sink.(Shutdowner); ok {
			if err := shutdowner.Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("shutdown sink %q: %w", worker.name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (w *sinkWorker) run() {
	defer close(w.done)
	for record := range w.queue {
		if err := w.sink.Write(context.Background(), record); err != nil {
			w.dropped.Add(1)
			w.errMu.Lock()
			w.lastErr = err
			w.errMu.Unlock()
		}
		w.pending.Add(-1)
	}
}

func (w *sinkWorker) waitDrained(ctx context.Context) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if w.pending.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
